<p align="center">
  <strong>toq SDK for Go</strong>
</p>

<p align="center">
  Go client for <a href="https://github.com/toqprotocol/toq">toq protocol</a>. Zero dependencies.
</p>

<p align="center">
  <a href="https://github.com/toqprotocol/toq-sdk-go/actions"><img src="https://github.com/toqprotocol/toq-sdk-go/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/toqprotocol/toq-sdk-go"><img src="https://pkg.go.dev/badge/github.com/toqprotocol/toq-sdk-go.svg" alt="Go Reference"></a>
  <a href="https://github.com/toqprotocol/toq-sdk-go/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="License"></a>
</p>

---

## Install

```bash
go get github.com/toqprotocol/toq-sdk-go
```

Requires Go 1.22+. Zero external dependencies.

## Prerequisites

1. Install the [toq binary](https://github.com/toqprotocol/toq)
2. Run `toq setup`
3. Run `toq up`

## Quick Start

```go
package main

import (
    "fmt"
    toq "github.com/toqprotocol/toq-sdk-go"
)

func main() {
    client := toq.Connect("")

    // Send a message
    resp, _ := client.Send("toq://192.168.1.50/bob", "Hey, are you available?", nil)
    fmt.Println(resp["thread_id"])

    // Check status
    status, _ := client.Status()
    fmt.Println(status["address"])

    // List peers
    peers, _ := client.Peers()
    for _, p := range peers {
        fmt.Printf("%s - %s\n", p["address"], p["status"])
    }

    // Stream incoming messages
    msgs, _ := client.Messages()
    for msg := range msgs {
        fmt.Printf("From %s: %v\n", msg.From, msg.Body)
        msg.Reply("Got it")
    }
}
```

### Streaming Delivery

```go
stream, _ := client.StreamStart("toq://192.168.1.50/bob", nil)
sid := stream["stream_id"].(string)
for _, word := range strings.Split("Hello from a streaming message", " ") {
    client.StreamChunk(sid, word+" ")
}
client.StreamEnd(sid, nil)
```

## URL Resolution

`Connect("")` resolves the daemon URL in this order:

1. Explicit URL: `Connect("http://127.0.0.1:9009")`
2. `TOQ_URL` environment variable
3. `.toq/state.json` in the current directory
4. `~/.toq/state.json`
5. Default: `http://127.0.0.1:9009`

## API

| Method | Description |
|--------|-------------|
| `Send(to, text, opts)` | Send a message |
| `Messages()` | Stream incoming messages (channel) |
| `StreamStart(to, opts)` | Open a streaming connection |
| `StreamChunk(id, text)` | Send a stream chunk |
| `StreamEnd(id, opts)` | End a stream |
| `GetThread(threadId)` | Get messages in a thread |
| `Peers()` | List known peers |
| `Block(key)` / `Unblock(key)` | Block/unblock by key or address |
| `Approvals()` | List pending approvals |
| `Approve(id)` / `Deny(id)` | Resolve an approval |
| `Revoke(id)` | Revoke an approved rule |
| `Permissions()` | List all permission rules |
| `Ping(address)` | Ping a remote agent |
| `History(opts)` | Query message history |
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
| `Handlers()` | List message handlers |
| `AddHandler(name, command, opts)` | Register a handler |
| `RemoveHandler(name)` | Remove a handler |
| `StopHandler(name, opts)` | Stop handler processes |

## Framework Plugins

For LangChain or CrewAI integration, see [toq-plugins](https://github.com/toqprotocol/toq-plugins) (Python).

## License

Apache 2.0
