package byte_rpc

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"
)

//const BODY_MAX_BYTES = 1000 * 100 //100KB

const MSG_TYPE_RESPONSE = "res"
const MSG_TYPE_REQUEST = "req"

const MSG_ERROR_CODE_NOERR = 0      //no error
const MSG_ERROR_CODE_NETWORK = 1    //network err
const MSG_ERROR_CODE_TIMEOUT = 2    //timeout error
const MSG_ERROR_CODE_PARSE = 3      //parse error
const MSG_ERROR_CODE_TYPE = 4       //type error
const MSG_ERROR_CODE_SEQ = 5        //sequence error
const MSG_ERROR_CODE_VER = 6        //version not match
const MSG_ERROR_CODE_METHOD = 7     //method error
const MSG_ERROR_CODE_MSGERRCODE = 8 //msg error code error
const MSG_ERROR_CODE_RESULT = 9     //msg result error
const MSG_ERROR_CODE_PARAM = 10     //param result error

//call message little endian
//type [3]byte
//sequence	uint64
//version uint16
//sub_version uint16
//method_len uint8
//method [method_len]byte
//param_len uint32
//param [param_len]byte

//response message little endian
//type [3]byte
//sequence	uint64
//msg_error_code uint16
//result_len uint32
//result [result_len]byte

type Handler func([]byte) []byte

type callback struct {
	done           chan struct{}
	msg_error_code uint16
	result         *[]byte
}

type Client struct {
	conn             io.ReadWriteCloser
	version          uint16
	sub_version      uint16
	sequence         uint64
	handlers         map[string]Handler
	send_mutex       sync.Mutex
	callbacks        map[uint64]*callback
	callbacks_mutex  sync.Mutex
	body_max_bytes   uint32
	method_max_bytes uint8
}

func NewClient(connection io.ReadWriteCloser, Version uint16, Sub_version uint16, body_max_bytes uint32, method_max_bytes uint8) *Client {
	return &Client{
		conn:             connection,
		version:          Version,
		sub_version:      Sub_version,
		sequence:         0,
		handlers:         make(map[string]Handler),
		callbacks:        make(map[uint64]*callback),
		body_max_bytes:   body_max_bytes,
		method_max_bytes: method_max_bytes,
	}
}

func (c *Client) Register(method string, handler Handler) {
	c.handlers[method] = handler
}

func (c *Client) Call(method string, param []byte) (*[]byte, uint16) {
	return c.Call_(method, param, time.Duration(30)*time.Second) //30 seconds timeout if no response
}

func (c *Client) Call_(method string, param []byte, timeout time.Duration) (*[]byte, uint16) {

	c.send_mutex.Lock()
	c.sequence++
	this_seq := c.sequence
	c.send_mutex.Unlock()

	//c.callbacks_mutex.Lock()
	c.callbacks[this_seq] = &callback{done: make(chan struct{}, 1), result: nil, msg_error_code: MSG_ERROR_CODE_TIMEOUT}
	//c.callbacks_mutex.Unlock()

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
		_, err = c.conn.Write([]byte(param))
		if err != nil {
			return
		}

	}()

	select {
	case <-c.callbacks[this_seq].done:

	case <-time.After(timeout):

	}

	var result *callback
	c.callbacks_mutex.Lock()
	result = c.callbacks[this_seq]
	delete(c.callbacks, this_seq)
	c.callbacks_mutex.Unlock()

	return result.result, result.msg_error_code
}

func (c *Client) Run() {
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
			if _, exist := c.handlers[string(method)]; !exist {
				c.respond(sequence, MSG_ERROR_CODE_METHOD, nil)
				break
			}

			//param
			param, p_err := c.read_param()
			if p_err != nil {
				c.respond(sequence, MSG_ERROR_CODE_PARAM, nil)
				break
			}

			//handle request
			result_byte := c.handlers[string(method)](param)
			err = c.respond(sequence, MSG_ERROR_CODE_NOERR, &result_byte)

			if err != nil || MSG_ERROR_CODE_NOERR > 0 {
				break
			}

		} else {
			//comes to the client
			msg_e_code, err := c.read_msg_error_code()
			if err != nil {
				c.write_callback(sequence, MSG_ERROR_CODE_MSGERRCODE, nil)
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

			err = c.write_callback(sequence, MSG_ERROR_CODE_NOERR, &result)
			if err != nil {
				break
			}

		}
	}

	//error happened
	c.conn.Close()
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

	if callback, exist := c.callbacks[sequence]; exist {
		callback.msg_error_code = msg_e_code
		callback.result = result
		callback.done <- struct{}{}
		return nil
	} else {
		return errors.New("callback not exist")
	}

}
