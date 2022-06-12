# BYTE_RPC

### a bi-directional Remote procedure call lib

### Multiplexing is supported internally by using an incremental sequence number


### you can easily define your serialization as this lib only deal with []byte


### request message    [`method` and `param` defined by user]
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


### response message  [`result` defined by user]
```
type [3]byte                
sequence	uint64          //little endian
msg_error_code uint16       //little endian
result_len uint32           //little endian
result [result_len]byte     
```