# toq SDK for Go

Go SDK for [toq protocol](https://github.com/toqprotocol/toq). Thin client to the local toq daemon. Zero dependencies.

## Install

```
go get github.com/toqprotocol/toq-sdk-go
```

## Prerequisites

1. Install the toq binary
2. Run `toq setup`
3. Run `toq up`

## Usage

```go
package main

import (
    "fmt"
    toq "github.com/toqprotocol/toq-sdk-go"
)

func main() {
    client := toq.Connect("")

    // Send a message
    resp, _ := client.Send("toq://peer.example.com/agent", "hello", nil)
    fmt.Println(resp["id"])

    // Receive messages
    msgs, _ := client.Messages()
    for msg := range msgs {
        fmt.Printf("From %s: %v\n", msg.From, msg.Body)
        msg.Reply("got it")
    }
}
```

## API

| Method | Description |
|--------|-------------|
| `Send(to, text, opts)` | Send a message |
| `Messages()` | Stream incoming messages (channel) |
| `CancelMessage(id)` | Cancel a sent message |
| `SendStreaming(to, text)` | Streaming delivery |
| `GetThread(threadId)` | Get messages in a thread |
| `Peers()` | List known peers |
| `Block(key)` / `Unblock(key)` | Block/unblock an agent |
| `Approvals()` | List pending approvals |
| `Approve(id)` / `Deny(id)` | Resolve an approval |
| `Discover(host)` / `DiscoverLocal()` | DNS/mDNS discovery |
| `Connections()` | List active connections |
| `Status()` / `Health()` | Daemon status |
| `Shutdown(graceful)` | Stop the daemon |
| `Logs()` / `ClearLogs()` | Read/clear logs |
| `FollowLogs()` | Stream logs in real time (channel) |
| `Diagnostics()` / `CheckUpgrade()` | Diagnostics |
| `RotateKeys()` | Rotate identity keys |
| `ExportBackup(passphrase)` | Create encrypted backup |
| `ImportBackup(passphrase, data)` | Restore from backup |
| `Config()` / `UpdateConfig(updates)` | Read/update config |
| `Card()` | Get agent card |

## License

Apache 2.0
