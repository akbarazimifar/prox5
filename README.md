## Prox5
[![GoDoc](https://godoc.org/git.tcp.direct/kayos/Prox5?status.svg)](https://godoc.org/git.tcp.direct/kayos/Prox5) [![Go Report Card](https://goreportcard.com/badge/github.com/yunginnanet/Prox5)](https://goreportcard.com/report/github.com/yunginnanet/Prox5) [![IRC](https://img.shields.io/badge/ircd.chat-%23tcpdirect-blue.svg)](ircs://ircd.chat:6697/#tcpdirect)
### SOCKS5/4/4a validating proxy pool

![Demo](./Prox5.gif)

This package is for managing and accessing thousands upon thousands of arbitrary SOCKS proxies. 

It also has a SOCKS5 server that dials out from ***?????*** Every time you ***?????***.

Pipe it a file filled with SOCKS proxies (host:port per line) and it will validate them continously while automatically weeding out the invalid ones.

This project is in development.

**See [the docs](https://godoc.org/git.tcp.direct/kayos/Prox5) and the [example](example/main.go) for more details.**
