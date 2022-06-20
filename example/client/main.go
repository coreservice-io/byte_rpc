package main

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/coreservice-io/byte_rpc"
	"github.com/coreservice-io/byte_rpc/example"
)

func main() {

	startClientServe()

	////connection
	conn, err := net.Dial("tcp", ":50000")
	if err != nil {
		fmt.Println("Error dialing", err.Error())
		return
	}

	client := byte_rpc.NewClient(conn, example.VERSION, example.SUB_VERSION, example.BODY_MAX_BYTES, example.METHOD_MAX_BYTES).Run()

	call_result, call_err := client.Call("build_conn", []byte("version x sub version xxx tcp port 8099"))
	fmt.Println("call_err", call_err)
	fmt.Println("call_result", string(*call_result))

	time.Sleep(1 * time.Hour)

}

func bindClient(connection io.ReadWriteCloser) *byte_rpc.Client {
	client := byte_rpc.NewClient(connection, example.VERSION, example.SUB_VERSION, example.BODY_MAX_BYTES, example.METHOD_MAX_BYTES).
		StartLivenessCheck(5*time.Second, func(err error) {
			fmt.Println("liveness check error:", err)
		})

	client.Register("hello", func(input []byte) []byte {
		fmt.Println("call to client hello param:", string(input))
		return []byte("hello :" + "client response from client")
	})

	return client
}

func startClientServe() {

	///////////////////////
	fmt.Println("Starting client serve ...")

	listener, err := net.Listen("tcp", ":50001")
	if err != nil {
		fmt.Println("Error listening", err.Error())
		return
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Println("Error accepting", err.Error())
				continue
			}
			/////
			bindClient(conn).Run()
		}
	}()

}
