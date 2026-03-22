# go-max-client

Неофициальная клиентская библиотека для [Max мессенджера](https://max.ru) на Go. Работает через WebSocket API, как веб-версия Max.

## Возможности

- Аутентификация по токену и QR-коду
- Отправка, редактирование, удаление и закрепление сообщений
- Получение истории сообщений
- Загрузка файлов: фото, видео, документы
- Управление группами: создание, вступление, выход, список участников
- Работа с каналами: информация, поиск по ссылке
- Получение профилей пользователей
- Изменение профиля и настроек
- Звонки: инициирование, приём, двусторонний поток данных через ICE
- Автоматический реконнект с экспоненциальным backoff
- Keepalive для поддержания соединения

## Установка

```
go get github.com/MrCatchParkington/go-max-client
```

## Примеры

### Подключение и аутентификация по токену

```go
package main

import (
	"context"
	"log"

	"github.com/MrCatchParkington/go-max-client/maxclient"
)

func main() {
	client := maxclient.New(
		maxclient.WithAutoReconnect(true),
	)
	defer client.Close()

	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}

	if err := client.AuthToken(ctx, "ваш-токен", "ваш-device-id"); err != nil {
		log.Fatal(err)
	}

	// Отправка сообщения
	_, err := client.SendMessage(ctx, 123456789, "Привет из go-max-client!")
	if err != nil {
		log.Fatal(err)
	}
}
```

### Аутентификация по QR-коду

```go
client := maxclient.New()
defer client.Close()

ctx := context.Background()

if err := client.Connect(ctx); err != nil {
	log.Fatal(err)
}

// Получаем ссылку для QR-кода
qrLink, err := client.StartQRAuth(ctx)
if err != nil {
	log.Fatal(err)
}
fmt.Println("Отсканируйте QR-код:", qrLink)

// Ждём сканирования
auth, err := client.WaitQRAuth(ctx)
if err != nil {
	log.Fatal(err)
}

// auth.Token и auth.DeviceID можно сохранить для последующей аутентификации через AuthToken
fmt.Printf("Токен: %s\nDeviceID: %s\n", auth.Token, auth.DeviceID)
```

### Отправка фото

```go
file, err := os.Open("photo.jpg")
if err != nil {
	log.Fatal(err)
}
defer file.Close()

attach, err := client.UploadPhoto(ctx, "photo.jpg", file)
if err != nil {
	log.Fatal(err)
}

_, err = client.SendMessage(ctx, chatID, "Подпись к фото", maxclient.SendMessageOpts{
	Attaches: []maxclient.Attachment{*attach},
})
```

### Получение входящих сообщений

```go
for pkt := range client.Packets() {
	if pkt.Opcode == maxclient.OpcodeMessageEvent {
		evt, err := maxclient.ParseMessageEvent(pkt)
		if err != nil {
			continue
		}
		fmt.Printf("[%d] %s\n", evt.ChatID, evt.Message.Text)
	}
}
```

## API

### Клиент

| Функция | Описание |
|---------|----------|
| `New(opts ...Option)` | Создание клиента |
| `Connect(ctx)` | Подключение к серверу |
| `Close()` | Закрытие соединения |
| `Packets()` | Канал входящих пакетов |
| `Errors()` | Канал ошибок соединения |
| `InvokeMethod(ctx, opcode, payload)` | Отправка произвольного запроса |

### Опции

| Опция | Описание |
|-------|----------|
| `WithAutoReconnect(bool)` | Автоматический реконнект при разрыве соединения |
| `WithReconnectBackoff(min, max)` | Интервалы между попытками реконнекта |
| `WithUserAgent(string)` | Пользовательский User-Agent |
| `WithLogger(*slog.Logger)` | Логгер |
| `WithPacketBufferSize(int)` | Размер буфера канала пакетов |
| `WithHTTPClient(*http.Client)` | HTTP-клиент для загрузки файлов и Calls API |

### Аутентификация

| Метод | Описание |
|-------|----------|
| `AuthToken(ctx, token, deviceID)` | Аутентификация по токену |
| `StartQRAuth(ctx)` | Начало QR-аутентификации, возвращает ссылку |
| `WaitQRAuth(ctx)` | Ожидание сканирования QR-кода |

### Сообщения

| Метод | Описание |
|-------|----------|
| `SendMessage(ctx, chatID, text, opts...)` | Отправка сообщения |
| `EditMessage(ctx, chatID, messageID, text, attaches...)` | Редактирование сообщения |
| `DeleteMessage(ctx, chatID, messageIDs, forMe)` | Удаление сообщений |
| `PinMessage(ctx, chatID, messageID, notify)` | Закрепление сообщения |
| `GetHistory(ctx, chatID, count)` | История сообщений |

### Загрузка файлов

| Метод | Описание |
|-------|----------|
| `UploadPhoto(ctx, filename, reader)` | Загрузка фото |
| `UploadVideo(ctx, filename, reader)` | Загрузка видео (ожидает обработку на сервере) |
| `UploadFile(ctx, filename, reader)` | Загрузка файла (ожидает обработку на сервере) |

Все методы загрузки возвращают `*Attachment`, который передаётся в `SendMessage` через `SendMessageOpts.Attaches`.

### Группы

| Метод | Описание |
|-------|----------|
| `CreateGroup(ctx, name, memberIDs)` | Создание группы |
| `GetGroupMembers(ctx, groupID)` | Список участников |
| `JoinGroup(ctx, groupID)` | Вступление в группу |
| `LeaveGroup(ctx, groupID)` | Выход из группы |

### Каналы

| Метод | Описание |
|-------|----------|
| `ResolveChannel(ctx, channelID)` | Информация о канале |
| `ResolveByLink(ctx, link)` | Поиск канала или группы по ссылке |

### Контакты

| Метод | Описание |
|-------|----------|
| `FindUserByPhone(ctx, phone)` | Поиск пользователя по номеру телефона, возвращает `*User` с `ID` (chatID) и `ExternalID` |

### Звонки

| Метод | Описание |
|-------|----------|
| `Call(ctx, calleeExternalID, forceRelay)` | Инициирование звонка (`calleeExternalID` — `int64` из `User.ExternalID`), возвращает `*CallSession` |
| `WaitForCall(ctx, forceRelay)` | Ожидание входящего звонка, возвращает `*CallSession` |

`CallSession` реализует `io.ReadWriteCloser` — двусторонний поток данных.

### Пользователи и профиль

| Метод | Описание |
|-------|----------|
| `ResolveUsers(ctx, userIDs)` | Получение профилей пользователей |
| `SetProfile(ctx, opts)` | Изменение своего профиля |
| `SetSettings(ctx, opts)` | Изменение настроек |

## Лицензия

[AGPL-3.0-or-later](LICENSE)
