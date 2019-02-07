package proxy

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/aarzilli/golua/lua"
)

// LuaEngine allows for analysis and modification of requests using Lua scripts
type LuaEngine struct {
	state *lua.State
}

// NewLuaEngine creates a new Lua engine (including the Lua stack and global state)
func NewLuaEngine() *LuaEngine {
	engine := &LuaEngine{
		state: lua.NewState(),
	}

	engine.state.OpenLibs()

	return engine
}

// Close cleans up the Lua stack and discards global state
func (e *LuaEngine) Close() {
	e.state.Close()
}

// DoFile executes a .lua file on the current state
func (e *LuaEngine) DoFile(filename string) error {
	return e.state.DoFile(filename)
}

// CallOnRequest executes the "onRequest" callback within the global scope
func (e *LuaEngine) CallOnRequest(r *http.Request) error {
	// wrap callOnRequest in case we want to introduce
	// lua_newthread later
	return callOnRequest(e.state, r)
}

// CallOnResponse executes the "onResponse" callback within the global scope
func (e *LuaEngine) CallOnResponse(r *http.Response) error {
	return callOnResponse(e.state, r)
}

func callOnRequest(L *lua.State, request *http.Request) error {
	L.CheckStack(4)

	// push (potential) function "onRequest" on stack
	L.GetGlobal("onRequest")

	if L.IsNoneOrNil(-1) {
		L.Pop(1)
		// TODO: decide whether this is an error or a NOP
		return fmt.Errorf("onRequest is undefined")
	} else if !L.IsFunction(-1) {
		L.Pop(1)
		return fmt.Errorf("onRequest is not a function")
	}

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return fmt.Errorf("Cannot read body: %v", err)
	}

	L.PushString(request.URL.String())
	pushHeader(L, request.Header)

	if len(body) > 0 {
		L.PushBytes(body)
	} else {
		// workaround, as lua_pushlstring does not work well if len == 0
		L.PushString("")
	}

	err = L.Call(3, 3)
	if err != nil {
		// TODO: what shall we do with the stack here?
		return fmt.Errorf("Error in onRequest: %v", err)
	}

	// url, headers, body (on stack in reversed order)
	if !L.IsString(-3) || !L.IsTable(-2) || !L.IsString(-1) {
		L.Pop(3)
		return fmt.Errorf("Wrong return value in onRequest()")
	}

	request.URL, err = url.Parse(L.ToString(-3))
	if err != nil {
		L.Pop(3)
		return fmt.Errorf("onRequest did not return a proper (parseable) URL: %v", err)
	}

	request.Header, err = toHeaders(L, -2)
	if err != nil {
		L.Pop(3)
		return fmt.Errorf("Could not parse headers returned by onRequest: %v", err)
	}

	request.Body = ioutil.NopCloser(bytes.NewReader(L.ToBytes(-1)))

	L.Pop(3)

	return nil
}

func callOnResponse(L *lua.State, response *http.Response) error {
	L.CheckStack(4)

	// push (potential) function "onRequest" on stack
	L.GetGlobal("onResponse")

	if L.IsNoneOrNil(-1) {
		L.Pop(1)
		// TODO: decide whether this is an error or a NOP
		return fmt.Errorf("onResponse is undefined")
	} else if !L.IsFunction(-1) {
		L.Pop(1)
		return fmt.Errorf("onResponse is not a function")
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("Cannot read body: %v", err)
	}
	// TODO: in case of error, restore body (attach new reader)

	L.PushInteger(int64(response.StatusCode))
	pushHeader(L, response.Header)

	if len(body) > 0 {
		L.PushBytes(body)
	} else {
		// workaround, as lua_pushlstring does not work well if len == 0
		L.PushString("")
	}

	err = L.Call(3, 3)
	if err != nil {
		// TODO: what shall we do with the stack here?
		return fmt.Errorf("Error in onResponse: %v", err)
	}

	// status, url, body (on stack in reversed order)
	if !L.IsNumber(-3) || !L.IsTable(-2) || !L.IsString(-1) {
		L.Pop(3)
		return fmt.Errorf("Wrong return value in onResponse()")
	}

	response.StatusCode = int(L.ToNumber(-3))

	response.Header, err = toHeaders(L, -2)
	if err != nil {
		L.Pop(3)
		return fmt.Errorf("Could not parse headers returned by onRequest: %v", err)
	}

	response.Body = ioutil.NopCloser(bytes.NewReader(L.ToBytes(-1)))

	L.Pop(3)

	return nil
}

func pushHeader(L *lua.State, headers map[string][]string) {
	L.CreateTable(len(headers), 0)

	for headerName, headerValues := range headers {
		L.CreateTable(0, len(headerValues))
		for i, headerValue := range headerValues {
			L.PushString(headerValue)
			L.RawSeti(-2, i+1) // note that Lua indexing is 1 based
		}
		L.SetField(-2, headerName)
	}
}

func toHeaders(L *lua.State, index int) (map[string][]string, error) {
	headers := make(map[string][]string)

	if index < 0 {
		index = L.GetTop() + index + 1
	}

	if !L.IsTable(index) {
		return headers, fmt.Errorf("Lua headers object is not a table")
	}

	// from https://pgl.yoyo.org/luai/i/lua_next

	// push nil as first key
	L.PushNil()

	// iterate: pop key, advance, push headerName and headerValue(s)
	for L.Next(index) != 0 {
		headerName := L.ToString(-2)
		// headerValues is now at stack index -1

		if L.IsString(-1) {
			headerValue := L.ToString(-1)
			headers[headerName] = []string{headerValue}
		} else if L.IsTable(-1) {
			// push nil as first key
			L.PushNil()

			// loop through headerValues (on stack below nil)
			for L.Next(-2) != 0 {
				if L.IsString(-1) {
					headerValue := L.ToString(-1)
					headers[headerName] = append(headers[headerName], headerValue)
				} else {
					// ignore other data types in list
				}

				L.Pop(1) // pop value, keep key for next iteration
			}
		}

		L.Pop(1) // pop value, keep headerName as key for next iteration
	}

	return headers, nil
}
