package hooks

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/d5/tengo/script"
	"github.com/d5/tengo/stdlib"
	"github.com/fd0/osmosis/proxy"
)

// CompileTengoPreHookFile is a CompileTengoPreHook wrapper that sets the script name and
// code based on the given file name and file content.
func CompileTengoPreHookFile(fileName string) (func(*proxy.Event) (*proxy.Response, error), error) {
	rawScript, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("reading script `%s`: %v", fileName, err)
	}
	return CompileTengoPreHook(fileName, rawScript)
}

// func HotReloadingTengoPreHook(fileName string) func(*proxy.Event) (*proxy.Response, error) {
// 	var scriptTemplate script.Compiled
// 	go func() {
// 		var oldHash []byte
// 		for {
// 			rawScript, err := ioutil.ReadFile(fileName)
// 			newHash := sha1.Sum(rawScript)
// 			if bytes.Equal(oldHash, newHash[:]) {
// 				time.Sleep(500 * time.Millisecond)
// 				continue
// 			}
// 			oldHash = newHash[:]
// 			if err != nil {
// 				fmt.Printf("reading script `%s`: %v\n", fileName, err)
// 				break
// 			}
// 			tmpl, err := prepareTengoPreScript(rawScript)
// 			if err != nil {
// 				fmt.Printf("setting up pre-script `%s`: %v\n", fileName, err)
// 			}
// 			fmt.Printf("compiled `%s` successfully\n", fileName)
// 			scriptTemplate = *tmpl
// 		}
// 	}()
// 	return tengoPreHook(fileName, &scriptTemplate)
// }

// CompileTengoPreHook compiles a Tengo script into a proxy hook that runs before a request is
// forwarded. In the script, the raw request is available through the Bytes variable `request`.
// If the script declares the Bytes variable `newRequest`, the original is replaced by the
// parsed value of this variable.
func CompileTengoPreHook(name string, rawScript []byte) (func(*proxy.Event) (*proxy.Response, error), error) {
	compiledScript, err := prepareTengoPreScript(rawScript)
	if err != nil {
		return nil, fmt.Errorf("setting up pre-script `%s`: %v", name, err)
	}
	return tengoPreHook(name, compiledScript), nil
}

func prepareTengoPreScript(code []byte) (*script.Compiled, error) {
	script := script.New(code)
	// scripts are trusted so we allow the whole standard library
	script.SetImports(stdlib.GetModuleMap(stdlib.AllModuleNames()...))
	err := script.Add("request", []byte{})
	if err != nil {
		return nil, fmt.Errorf("adding request: %v", err)
	}

	compiledScript, err := script.Compile()
	if err != nil {
		return nil, fmt.Errorf("compile error: %v", err)
	}
	return compiledScript, nil
}

func tengoPreHook(name string, scriptTemplate *script.Compiled) func(*proxy.Event) (*proxy.Response, error) {
	return func(event *proxy.Event) (*proxy.Response, error) {
		if scriptTemplate == nil {
			event.Log("pre-hook `%s` is no-op", name)
			return event.ForwardRequest()
		}
		scriptInstance := scriptTemplate.Clone()

		rawRequest, err := event.RawRequest()
		if err != nil {
			return nil, fmt.Errorf("dumping request for tengo pre-script `%s`: %v", name, err)
		}

		err = scriptInstance.Set("request", rawRequest)
		if err != nil {
			return nil, fmt.Errorf("retting pre-script `%s` request var: %v", name, err)
		}

		err = scriptInstance.Run()
		if err != nil {
			return nil, fmt.Errorf("runtime error in pre-script `%s`: %v", name, err)
		}

		if scriptInstance.IsDefined("request") {
			newRawRequest := scriptInstance.Get("request").Bytes()
			if newRawRequest == nil {
				return nil, fmt.Errorf("pre-script `%s`: newRequest is not of type Bytes", name)
			}

			if !bytes.Equal(rawRequest, newRawRequest) {
				err = event.SetRequest(newRawRequest)
				if err != nil {
					event.Log(string(newRawRequest))
					return nil, fmt.Errorf("updating request after pre-script `%s`: %v", name, err)
				}
			}
		} else {
			event.Log("newRequest not defined, request is not updated")
		}

		return event.ForwardRequest()
	}
}

// CompileTengoPostHookFile is a CompileTengoPostHook wrapper that sets the script name and
// code based on the given file name and file content.
func CompileTengoPostHookFile(fileName string) (func(*proxy.Event) (*proxy.Response, error), error) {
	rawScript, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %v", fileName, err)
	}
	return CompileTengoPostHook(fileName, rawScript)
}

// CompileTengoPostHook compiles a Tengo script into a proxy hook that runs after the response
// is received. In the script, the raw response as well as the request are available through
// the Bytes variables `response` and `request`. If the script declares the Bytes variable
// `newResponse`, the original is replaced by the parsed value of this variable.
func CompileTengoPostHook(name string, rawScript []byte) (func(*proxy.Event) (*proxy.Response, error), error) {
	compiledScript, err := prepareTengoPostScript(rawScript)
	if err != nil {
		return nil, fmt.Errorf("setting up post-script `%s`: %v", name, err)
	}
	return tengoPostHook(name, compiledScript), nil
}

func prepareTengoPostScript(code []byte) (*script.Compiled, error) {
	script := script.New(code)
	// scripts are trusted so we allow the whole standard library
	script.SetImports(stdlib.GetModuleMap(stdlib.AllModuleNames()...))
	err := script.Add("request", []byte{})
	if err != nil {
		return nil, fmt.Errorf("adding request: %v", err)
	}
	err = script.Add("response", []byte{})
	if err != nil {
		return nil, fmt.Errorf("adding response: %v", err)
	}
	compiledScript, err := script.Compile()
	if err != nil {
		return nil, fmt.Errorf("compile error: %v", err)
	}

	return compiledScript, nil
}

func tengoPostHook(name string, scriptTemplate *script.Compiled) func(*proxy.Event) (*proxy.Response, error) {
	return func(event *proxy.Event) (*proxy.Response, error) {
		response, err := event.ForwardRequest()
		if err != nil {
			return nil, err
		}

		if scriptTemplate == nil {
			event.Log("post-hook `%s` is no-op", name)
			return response, nil
		}

		scriptInstance := scriptTemplate.Clone()

		rawRequest, err := event.RawRequest()
		if err != nil {
			return nil, fmt.Errorf("dumping request for tengo post-script `%s`: %v", name, err)
		}

		rawResponse, err := response.Raw()
		if err != nil {
			return nil, fmt.Errorf("dumping response for tengo post-script `%s`: %v", name, err)
		}

		err = scriptInstance.Set("request", rawRequest)
		if err != nil {
			return nil, fmt.Errorf("setting post-script `%s` request var: %v", name, err)
		}
		err = scriptInstance.Set("response", rawResponse)
		if err != nil {
			return nil, fmt.Errorf("setting post-script `%s` response var: %v", name, err)
		}

		err = scriptInstance.Run()
		if err != nil {
			return nil, fmt.Errorf("runtime error in post-script `%s`: %v", name, err)
		}

		if !scriptInstance.IsDefined("response") {
			return nil, fmt.Errorf("post-script `%s` response variable is not defined", name)
		}

		newRawResponse := scriptInstance.Get("response").Bytes()
		if newRawResponse == nil {
			return nil, fmt.Errorf("post-script `%s`: newResponse is not of type Bytes", name)
		}

		if !bytes.Equal(rawResponse, newRawResponse) {
			err = response.Set(newRawResponse)
			if err != nil {
				event.Log(string(newRawResponse))
				return nil, fmt.Errorf("updating response after post-script `%s`: %v", name, err)
			}
		}

		return response, nil
	}
}
