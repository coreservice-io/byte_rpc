package main

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/coreservice-io/byte_rpc"
	"github.com/coreservice-io/byte_rpc/example"
)

///func bindServerConn()

func bindBuildConn(connection io.ReadWriteCloser) *byte_rpc.Client {
	client := byte_rpc.NewClient(connection, example.VERSION, example.SUB_VERSION, example.BODY_MAX_BYTES, example.METHOD_MAX_BYTES)
	client.Register("build_conn", func(input []byte) []byte {
		fmt.Println("build_conn:", string(input))
		go buildConn()
		return []byte("build_conn approved")
	})

	time.AfterFunc(15*time.Second, func() {
		fmt.Println("15 seconds passed and conn to be closed")
		client.Close()
	})

	return client
}

func buildConn() {
	////connection
	conn, err := net.Dial("tcp", "localhost:50001")
	if err != nil {
		fmt.Println("build conn Error dialing", err.Error())
		return
	}

	client := byte_rpc.NewClient(conn, example.VERSION, example.SUB_VERSION, example.BODY_MAX_BYTES, example.METHOD_MAX_BYTES)

	client.StartLivenessCheck(time.Duration(10*time.Second), func(err error) {
		fmt.Println("live error:", err)
	})

	go client.Run()

	call_result, call_err := client.Call("hello", []byte("call hello from server"))
	fmt.Println("hello call_err", call_err)
	fmt.Println("hello call_result", string(*call_result))

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

		///// cache ip hit, for the server callback that i am tring to build connection with
		//go bindServerConn(conn).Run()

		/////for the client who is tring to build a connection to me
		go bindBuildConn(conn).Run()

	}
}
