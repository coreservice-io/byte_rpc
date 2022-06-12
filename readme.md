# BYTE_RPC

### a bi-directional Remote procedure call lib

#### Multiplexing is supported internally by using an incremental sequence number
#### you can use your serialization method as this lib only handle []byte


#### request message    [`method` and `param` defined by user]
```
type [3]byte
sequence	uint64          //little endian
version uint16              //little endian
sub_version uint16          //little endian
method_len uint8 
method [method_len]byte 
param_len uint32            //little endian
param [param_len]byte
```


#### response message  [`result` defined by user]
```
type [3]byte                
sequence	uint64          //little endian
msg_error_code uint16       //little endian
result_len uint32           //little endian
result [result_len]byte     
```




### server


```go
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

```


### client

```go
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

```