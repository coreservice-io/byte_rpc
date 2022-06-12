package main

import (
	"bytes"
	"fmt"
	"net"
	"time"

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

	i := 0
	for {
		i++
		if i == 30 {
			client.Close()
			time.Sleep(10 * time.Second)
			break
		}
		go rand_call(client)
		time.Sleep(1 * time.Second)
	}

}

func rand_call(c *byte_rpc.Client) {

	for i := 1; i < 1000000; i++ {
		param := []byte{9, 9, 9, 9, 9}
		call_result, call_err := c.Call("helloz", param)
		fmt.Println("call_err", call_err)
		fmt.Println("call_result", call_result)
		if call_result != nil && bytes.Compare(param, *call_result) != 0 {
			panic("error")
		}
		fmt.Println(time.Now().UTC())

		param2 := []byte{1, 1, 1, 1, 1}
		hellow_result, hellow_result_err := c.Call("hellow", param2)
		fmt.Println("call_err", hellow_result_err)
		fmt.Println("call_result", hellow_result)
		fmt.Println(time.Now().UTC())
		if hellow_result != nil && bytes.Compare(param2, *hellow_result) != 0 {
			panic("error")
		}
	}

}
