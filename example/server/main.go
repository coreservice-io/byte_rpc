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

	client.Register("hello", func(input []byte) []byte {
		fmt.Println("hello param:", string(input))

		name_from_client, err_code := client.Call("name", nil)
		fmt.Println("name_from_client", string(*name_from_client))
		fmt.Println("name_from_client err_code ", err_code)

		return []byte("hello :" + string(*name_from_client))
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
