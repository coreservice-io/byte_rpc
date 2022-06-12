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

		call_from_server, err_code := client.Call("hello", []byte{1, 2, 3})
		fmt.Println("call_from_server", call_from_server)
		fmt.Println("call_from_server err_code ", err_code)

		call_from_serverx, err_codex := client.Call("hellox", []byte{1, 2, 3})
		fmt.Println("call_from_serverx", call_from_serverx)
		fmt.Println("call_from_serverx err_code ", err_codex)

		return b
	})

	client.Register("hellow", func(b []byte) []byte {
		fmt.Println("hellow_param:", b)
		//return []byte{12, 13, 14, 15}
		return b
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
