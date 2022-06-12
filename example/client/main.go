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

	client.Register("name", func(b []byte) []byte {
		return []byte("jack")
	})

	go client.Run()

	call_result, call_err := client.Call("hello", []byte("hello server"))
	fmt.Println("call_err", call_err)
	fmt.Println("call_result", string(*call_result))

}
