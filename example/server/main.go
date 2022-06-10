package main

import (
	"fmt"
	"io"
	"net"

	"github.com/coreservice-io/byte_rpc"
	"github.com/coreservice-io/byte_rpc/example"
)

func bindClient(connection io.ReadWriteCloser) *byte_rpc.Client {
	client := byte_rpc.NewClient(connection, example.VERSION, example.SUB_VERSION, example.BODY_MAX_BYTES, example.METHOD_MAX_BYTES)

	client.Register("helloz", func(b []byte) []byte {
		fmt.Println("helloz_param:", b)
		return []byte{8, 9, 10, 11}
	})

	client.Register("hellow", func(b []byte) []byte {
		fmt.Println("hellow_param:", b)
		return []byte{12, 13, 14, 15}
	})

	return client
}

func main() {

	///////////////////////
	fmt.Println("Starting the server ...")

	listener, err := net.Listen("tcp", "localhost:50000")
	if err != nil {
		fmt.Println("Error listening", err.Error())
		return
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting", err.Error())
			continue
		}

		/////
		go bindClient(conn).Run()
	}
}
