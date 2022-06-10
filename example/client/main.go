package main

import (
	"fmt"
	"net"

	"github.com/coreservice-io/byte_rpc"
	"github.com/coreservice-io/byte_rpc/example"
)

func main() {

	////connection
	conn, err := net.Dial("tcp", "localhost:50000")
	if err != nil {
		fmt.Println("Error dialing", err.Error())
		return
	}

	client := byte_rpc.NewClient(conn, example.VERSION, example.SUB_VERSION, example.BODY_MAX_BYTES, example.METHOD_MAX_BYTES)

	client.Register("hello", func(b []byte) []byte {
		fmt.Println("hello_param:", b)
		return []byte{0, 1, 2, 3}
	})

	client.Register("hellox", func(b []byte) []byte {
		fmt.Println("hellox_param:", b)
		return []byte{4, 5, 6, 7}
	})

	go client.Run()

	call_result, call_err := client.Call("helloz", []byte{9, 9, 9, 9})
	fmt.Println("call_err", call_err)
	fmt.Println("call_result", call_result)

}
