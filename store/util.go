package store

import (
	"bufio"
	"bytes"
	"net/http"

	"github.com/dgraph-io/badger"
)

func valueBufioReader(item *badger.Item) (*bufio.Reader, error) {
	reqBytes, err := item.Value()
	if err != nil {
		return nil, err
	}
	return bufio.NewReader(bytes.NewReader(reqBytes)), nil
}

func parseRequest(item *badger.Item) (*http.Request, error) {
	reader, err := valueBufioReader(item)
	if err != nil {
		return nil, err
	}
	return http.ReadRequest(reader)
}

func parseResponse(item *badger.Item) (*http.Response, error) {
	reader, err := valueBufioReader(item)
	if err != nil {
		return nil, err
	}
	// TODO: also add body
	return http.ReadResponse(reader, nil)
}
