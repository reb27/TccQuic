# TCC
QUIC client and server

## Client
TODO

## Server
Server implements the Queue policies
WFQ: TODO
SP: TODO

## Setup Instructions
Install GO version 1.18.x or 1.19.x (quic-go will not work on 1.20, see https://github.com/quic-go/quic-go/wiki/quic-go-and-Go-versions and https://pkg.go.dev/github.com/lucas-clemente/quic-go#section-readme for more details): https://go.dev/dl/
Clone repository: https://github.com/quic-streaming/tcc

## How to run
Start Server:
`go run main.go server wfq` will start the server with the Weighted Fair Queue policy
`go run main.go server wfq` will start the server with the Strict Priority Queue policy

Start Client: 
`go run main.go client` will start the client

