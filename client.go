package byte_rpc

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"
)

// ### request message    [`method` and `param` defined by user]
// type [3]byte
// sequence	uint64			   //little endian
// version uint16              //little endian
// sub_version uint16          //little endian
// method_len uint8
// method [method_len]byte
// param_len uint32            //little endian
// param [param_len]byte

// ### response message  [`result` defined by user]
// type [3]byte
// sequence	uint64             //little endian
// msg_error_code uint16       //little endian
// result_len uint32           //little endian
// result [result_len]byte

const MSG_TYPE_RESPONSE = "res"
const MSG_TYPE_REQUEST = "req"

const MSG_ERROR_CODE_NO_ERR = 0     //no error
const MSG_ERROR_CODE_CONN_CLOSE = 1 //conn close err
const MSG_ERROR_CODE_TIMEOUT = 2    //timeout error
const MSG_ERROR_CODE_VER = 3        //version not match
const MSG_ERROR_CODE_METHOD = 4     //method error
const MSG_ERROR_CODE_MSG_CODE = 5   //msg error code error
const MSG_ERROR_CODE_RESULT = 6     //msg result error
const MSG_ERROR_CODE_PARAM = 7      //param  error

func GetErrMsgStr(err_code uint) string {

	var err_msg_map = map[uint]string{
		1: "conn closed error",
		2: "timeout error",
		3: "version not match error",
		4: "method error",
		5: "error code error",
		6: "result error",
		7: "param error",
	}

	return err_msg_map[err_code]
}

const LIVE_FAIL_THRESHOLD = 4 // LIVE_FAIL_THRESHOLD * (ping/pong interval) no response

type Handler func([]byte) []byte

type callback struct {
	done           chan struct{}
	msg_error_code uint16
	result         *[]byte
}

type Client struct {
	conn                 io.ReadWriteCloser
	conn_closed          bool
	version              uint16
	sub_version          uint16
	sequence             uint64
	handlers             sync.Map //map[string]Handler
	send_mutex           sync.Mutex
	callbacks            sync.Map //map[uint64]*callback
	callbacks_mutex      sync.Mutex
	body_max_bytes       uint32
	method_max_bytes     uint8
	last_live_unixtime   int64 //last live check time
	conn_closed_callback func(error)
	live_check_duration  time.Duration
}

type Config struct {
	Version              uint16
	Sub_version          uint16
	Body_max_bytes       uint32
	Method_max_bytes     uint8
	Live_check_duration  time.Duration
	Conn_closed_callback func(error)
}

//live_check_duration is used for a background ping/pong routine
func NewClient(connection io.ReadWriteCloser, config *Config) *Client {
	client := &Client{
		conn:                 connection,
		conn_closed:          false,
		version:              config.Version,
		sub_version:          config.Sub_version,
		sequence:             0,
		handlers:             sync.Map{}, //make(map[string]Handler),
		callbacks:            sync.Map{}, //make(map[uint64]*callback),
		body_max_bytes:       config.Body_max_bytes,
		method_max_bytes:     config.Method_max_bytes,
		last_live_unixtime:   0,
		conn_closed_callback: config.Conn_closed_callback,
		live_check_duration:  config.Live_check_duration,
	}

	//config the ping/pong routing
	client.Register("ping", func(b []byte) []byte {
		return []byte("pong")
	})

	return client
}

func (c *Client) StartLivenessCheck() *Client {

	c.last_live_unixtime = time.Now().Unix()

	go func() {
		for {
			time.Sleep(c.live_check_duration)
			if time.Now().Unix()-c.last_live_unixtime > LIVE_FAIL_THRESHOLD*int64(c.live_check_duration.Seconds()) {
				c.Close()
				c.conn_closed_callback(errors.New("ping time out"))
				break
			}
		}
	}()

	//start the ping routing
	go func() {
		for {
			//some msg from the other end arrived during last sleep
			if c.last_live_unixtime < time.Now().Unix()-int64(c.live_check_duration.Seconds()) {
				_, call_err := c.Call("ping", nil)
				if call_err != 0 {
					c.Close()
					c.conn_closed_callback(errors.New("ping failure"))
					break
				}
				c.last_live_unixtime = time.Now().Unix()
			}
			time.Sleep(c.live_check_duration + 1)
		}
	}()

	return c

}

func (c *Client) Register(method string, handler Handler) error {
	if _, ok := c.handlers.Load(method); ok {
		return errors.New("method conflict:" + method)
	}
	c.handlers.Store(method, handler)
	return nil
}

func (c *Client) Call(method string, param []byte) (*[]byte, uint16) {
	return c.Call_(method, param, time.Duration(30)*time.Second) //30 seconds timeout if no response
}

func (c *Client) Call_(method string, param []byte, timeout time.Duration) (*[]byte, uint16) {

	c.send_mutex.Lock()
	if c.conn_closed {
		c.send_mutex.Unlock()
		return nil, MSG_ERROR_CODE_CONN_CLOSE
	}

	c.sequence++
	this_seq := c.sequence
	c.callbacks.Store(this_seq, &callback{done: make(chan struct{}, 2), result: nil, msg_error_code: MSG_ERROR_CODE_TIMEOUT})
	c.send_mutex.Unlock()

	//send msg
	go func() {
		c.send_mutex.Lock()
		defer c.send_mutex.Unlock()

		//type
		_, err := c.conn.Write([]byte(MSG_TYPE_REQUEST))
		if err != nil {
			return
		}

		//sequence
		sequence := make([]byte, 8)
		binary.LittleEndian.PutUint64(sequence, this_seq)
		_, err = c.conn.Write(sequence)
		if err != nil {
			return
		}

		//version
		version := make([]byte, 2)
		binary.LittleEndian.PutUint16(version, c.version)
		_, err = c.conn.Write(version)
		if err != nil {
			return
		}

		//sub_version
		sub_version := make([]byte, 2)
		binary.LittleEndian.PutUint16(sub_version, c.sub_version)
		_, err = c.conn.Write(sub_version)
		if err != nil {
			return
		}

		//method_len
		_, err = c.conn.Write([]byte{byte(uint8(len(method)))})
		if err != nil {
			return
		}

		//method
		_, err = c.conn.Write([]byte(method))
		if err != nil {
			return
		}

		//param_len
		param_len := make([]byte, 4)
		binary.LittleEndian.PutUint32(param_len, uint32(len(param)))
		_, err = c.conn.Write(param_len)
		if err != nil {
			return
		}

		//param
		if len(param) > 0 {
			_, err = c.conn.Write([]byte(param))
			if err != nil {
				return
			}
		}

	}()

	seq_callback, _ := c.callbacks.Load(this_seq)
	select {
	case <-seq_callback.(*callback).done:
	case <-time.After(timeout):
	}

	var result *callback
	c.callbacks_mutex.Lock()
	result_interface, _ := c.callbacks.LoadAndDelete(this_seq)
	result = result_interface.(*callback)
	c.callbacks_mutex.Unlock()

	return result.result, result.msg_error_code
}

func (c *Client) Run() *Client {
	go c.run_()
	return c
}

func (c *Client) run_() {

	//////main//////
	for {
		//type
		type_bytes := make([]byte, 3)
		vbyte, err := io.ReadFull(c.conn, type_bytes[:])
		if err != nil || vbyte != 3 || (string(type_bytes) != MSG_TYPE_RESPONSE && string(type_bytes) != MSG_TYPE_REQUEST) {
			//just break this tcp link
			break
		}

		sequence, err := c.read_sequence()
		if err != nil {
			//just break this tcp link
			break
		}

		c.last_live_unixtime = time.Now().Unix()

		if string(type_bytes) == MSG_TYPE_REQUEST {
			//comes to the server

			//version and sub version check
			_, _, v_err := c.read_version()
			if v_err != nil {
				c.respond(sequence, MSG_ERROR_CODE_VER, nil)
				break
			}

			//method
			method, m_err := c.read_method()
			if m_err != nil {
				c.respond(sequence, MSG_ERROR_CODE_METHOD, nil)
				break
			}

			handler_interface, ok := c.handlers.Load(string(method))
			if !ok {
				c.respond(sequence, MSG_ERROR_CODE_METHOD, nil)
				break
			}

			//param
			param, p_err := c.read_param()
			if p_err != nil {
				c.respond(sequence, MSG_ERROR_CODE_PARAM, nil)
				break
			}

			//process the process using go-routing and read directly the next income
			go func() {
				//handle request
				result_byte := handler_interface.(Handler)(param)
				err = c.respond(sequence, MSG_ERROR_CODE_NO_ERR, &result_byte)
				if err != nil || MSG_ERROR_CODE_NO_ERR > 0 {
					c.Close()
				}
			}()

		} else {
			//comes to the client
			msg_e_code, err := c.read_msg_error_code()
			if err != nil {
				c.write_callback(sequence, MSG_ERROR_CODE_MSG_CODE, nil)
				break
			}

			if msg_e_code > 0 {
				c.write_callback(sequence, msg_e_code, nil)
				break
			}

			//result
			result, result_err := c.read_result()
			if result_err != nil {
				c.write_callback(sequence, MSG_ERROR_CODE_RESULT, nil)
				break
			}

			err = c.write_callback(sequence, MSG_ERROR_CODE_NO_ERR, &result)
			if err != nil {
				break
			}

		}
	}

	//error happened
	c.Close()

}

func (c *Client) Close() {
	c.send_mutex.Lock()
	c.callbacks_mutex.Lock()
	if !c.conn_closed {

		c.callbacks.Range(func(k, v interface{}) bool {
			if v.(*callback).msg_error_code == MSG_ERROR_CODE_NO_ERR {
				v.(*callback).msg_error_code = MSG_ERROR_CODE_CONN_CLOSE
				c.callbacks.Store(k.(uint64), v.(*callback))
			}
			v.(*callback).done <- struct{}{}
			return true
		})

		c.conn.Close()
		c.conn_closed = true
	}
	c.callbacks_mutex.Unlock()
	c.send_mutex.Unlock()
	c.conn_closed_callback(nil)
}

func (c *Client) read_sequence() (uint64, error) {
	//sequence
	sequence_byte := make([]byte, 8)
	seq_byte, err := io.ReadFull(c.conn, sequence_byte[:])
	if err != nil || seq_byte != 8 {
		return 0, errors.New("sequence error")
	}
	return binary.LittleEndian.Uint64(sequence_byte), nil
}

func (c *Client) read_version() (uint16, uint16, error) {

	////version
	version := make([]byte, 2)
	vbyte, err := io.ReadFull(c.conn, version[:])
	if err != nil || vbyte != 2 {
		return 0, 0, errors.New("version error")
	}

	h_v := binary.LittleEndian.Uint16(version)
	if h_v != c.version {
		return 0, 0, errors.New("version error")
	}

	///sub_version
	sub_version := make([]byte, 2)
	subv_byte, err := io.ReadFull(c.conn, sub_version[:])
	if err != nil || subv_byte != 2 {
		return 0, 0, errors.New("sub_version error")
	}

	h_sub_b := binary.LittleEndian.Uint16(sub_version)

	return h_v, h_sub_b, nil

}

func (c *Client) read_method() ([]byte, error) {

	//method_len
	method_len_byte := make([]byte, 1)
	method_len_byte_count, err := io.ReadFull(c.conn, method_len_byte[:])
	if err != nil || method_len_byte_count != 1 {
		return nil, errors.New("method_len error")
	}

	method_len := uint8(method_len_byte[0])

	if method_len > c.method_max_bytes {
		return nil, errors.New("method_len error")
	}

	//method
	method := make([]byte, method_len)
	method_bytes, err := io.ReadFull(c.conn, method[:])
	if err != nil || method_bytes != int(method_len) {
		return nil, errors.New("method error")
	}

	return method, nil
}

func (c *Client) read_param() ([]byte, error) {

	//param_len
	param_len_byte := make([]byte, 4)
	param_len_byte_count, err := io.ReadFull(c.conn, param_len_byte[:])
	if err != nil || param_len_byte_count != 4 {
		return nil, errors.New("param_len error")
	}

	param_len := binary.LittleEndian.Uint32(param_len_byte)

	if param_len > c.body_max_bytes {
		return nil, errors.New("param_len error")
	}

	//param
	param := make([]byte, param_len)
	param_bytes, err := io.ReadFull(c.conn, param[:])
	if err != nil || param_bytes != int(param_len) {
		return nil, errors.New("param error")
	}

	return param, nil
}

func (c *Client) read_result() ([]byte, error) {

	//result_len
	result_len_byte := make([]byte, 4)
	result_len_byte_count, err := io.ReadFull(c.conn, result_len_byte[:])
	if err != nil || result_len_byte_count != 4 {
		return nil, errors.New("result_len error")
	}

	result_len := binary.LittleEndian.Uint32(result_len_byte)

	if result_len > c.body_max_bytes {
		return nil, errors.New("result_len error")
	}

	//result
	result := make([]byte, result_len)
	result_bytes, err := io.ReadFull(c.conn, result[:])
	if err != nil || result_bytes != int(result_len) {
		return nil, errors.New("result error")
	}

	return result, nil
}

func (c *Client) read_msg_error_code() (uint16, error) {
	//msg_error_code
	msg_error_code := make([]byte, 2)
	msg_error_code_byte, err := io.ReadFull(c.conn, msg_error_code[:])
	if err != nil || msg_error_code_byte != 2 {
		return 0, errors.New("msg_error_code error")
	}
	return binary.LittleEndian.Uint16(msg_error_code), nil
}

func (c *Client) respond(Sequence uint64, Error_code uint16, result *[]byte) error {

	c.send_mutex.Lock()
	defer c.send_mutex.Unlock()

	//type
	_, err := c.conn.Write([]byte(MSG_TYPE_RESPONSE))
	if err != nil {
		return err
	}

	//sequence
	sequence := make([]byte, 8)
	binary.LittleEndian.PutUint64(sequence, Sequence)
	_, err = c.conn.Write(sequence)
	if err != nil {
		return err
	}

	if Error_code > 0 {

		//write msg_err_code
		msg_err_code := make([]byte, 2)
		binary.LittleEndian.PutUint16(msg_err_code, Error_code)
		_, err = c.conn.Write(msg_err_code)
		if err != nil {
			return err
		}

		//write result_len
		result_len := make([]byte, 4)
		binary.LittleEndian.PutUint32(result_len, 0)
		_, err = c.conn.Write(result_len)
		if err != nil {
			return err
		}

	} else {

		//write msg_err_code
		msg_err_code := make([]byte, 2)
		binary.LittleEndian.PutUint16(msg_err_code, 0)
		_, err = c.conn.Write(msg_err_code)
		if err != nil {
			return err
		}

		//write result_len
		result_len := make([]byte, 4)
		binary.LittleEndian.PutUint32(result_len, uint32(len(*result)))
		_, err = c.conn.Write(result_len)
		if err != nil {
			return err
		}

		//write result
		if len(*result) > 0 {
			_, err = c.conn.Write(*result)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

func (c *Client) write_callback(sequence uint64, msg_e_code uint16, result *[]byte) error {
	c.callbacks_mutex.Lock()
	defer c.callbacks_mutex.Unlock()

	callback_interface, ok := c.callbacks.Load(sequence)
	if ok {
		callback_interface.(*callback).msg_error_code = msg_e_code
		callback_interface.(*callback).result = result
		callback_interface.(*callback).done <- struct{}{}
		return nil
	} else {
		return errors.New("callback not exist")
	}

}
