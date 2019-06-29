package store

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
)

const (
	req = `GET /doc/ HTTP/1.1
User-Agent: HTTPie/1.0.2
Accept: */*
Connection: keep-alive
Host: golang.org

`
	res = `HTTP/1.1 200 OK
Alt-Svc: quic=":443"; ma=2592000; v="46,44,43,39"
Content-Type: text/html; charset=utf-8
Date: Fri, 28 Jun 2019 16:44:43 GMT
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
Transfer-Encoding: chunked
Vary: Accept-Encoding
Via: 1.1 google

`
)

var testCases = []struct {
	body      string
	editedReq bool
	editedRes bool
	hasRes    bool
}{
	{body: "first", editedReq: false, editedRes: false, hasRes: false},
	{body: "second", editedReq: false, editedRes: false, hasRes: true},
	{body: "third", editedReq: false, editedRes: true, hasRes: true},
	{body: "fourth", editedReq: true, editedRes: false, hasRes: false},
	{body: "fifth", editedReq: true, editedRes: false, hasRes: true},
	{body: "sixth", editedReq: true, editedRes: false, hasRes: true},
	{body: "seventh", editedReq: true, editedRes: true, hasRes: true},
}

func TestStore(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "osmosis.testing.store.")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	mustSucceed := func(info string, err error) {
		if err != nil {
			t.Fatalf("adding item `%s` failed: %s", info, err)
		}
	}

	request, err := http.ReadRequest(bufio.NewReader(bytes.NewReader([]byte(req))))
	if err != nil {
		t.Fatalf("could not setup test request: %s", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader([]byte(res))), nil)
	if err != nil {
		t.Fatalf("could not setup test response: %s", err)
	}

	var store *TxnStore

	t.Run("NewTxnStore", func(t *testing.T) {
		store, err = NewTxnStore(dir)
		if err != nil {
			t.Fatalf("store creating failed: %s", err)
		}
	})

	t.Run("AddRequest", func(t *testing.T) {
		for i, tc := range testCases {
			mustSucceed(tc.body, store.AddRequest(uint64(i), request, false))
			if tc.editedReq {
				mustSucceed(tc.body, store.AddRequest(uint64(i), request, true))
			}
		}
	})

	t.Run("AddResponse", func(t *testing.T) {
		for i, tc := range testCases {
			if tc.hasRes {
				mustSucceed(tc.body, store.AddResponse(uint64(i), response,
					[]byte(tc.body), false))
				if tc.editedRes {
					mustSucceed(tc.body, store.AddResponse(uint64(i), response,
						[]byte(tc.body), true))
				}

			}
		}
	})

	t.Run("MaxID", func(t *testing.T) {
		maxID, err := store.MaxID()
		if err != nil {
			t.Fatalf("getting MaxID failed: %s", err)
		}
		wantedMaxID := uint64(len(testCases)) - 1

		if maxID != wantedMaxID {
			t.Fatalf("MaxID is %d (should be %d)", maxID, wantedMaxID)
		}
	})

	t.Run("TxnSummaries", func(t *testing.T) {
		summaries, err := store.TxnSummaries()
		if err != nil {
			t.Fatalf("TxnSummaries failed: %s", err)
		}
		wantedLength := len(testCases)

		if len(summaries) != wantedLength {
			t.Fatalf("TxnSummaries returned %d summaries (should return %d)", len(summaries), wantedLength)
		}

		for i, summary := range summaries {
			if summary.ID != uint64(i) {
				t.Fatalf("TxnSummaries[%[1]d] has wrong ID %[1]d (should be %[2]d)",
					summary.ID, i)
			}
			if summary.ReqEdited != testCases[i].editedReq {
				t.Fatalf("TxnSummaries[%d].ReqEdited is %t (should be %t)",
					i, summary.ReqEdited, testCases[i].editedReq)
			}
			if summary.ResEdited != testCases[i].editedRes {
				t.Fatalf("TxnSummaries[%d].ResEdited is %t (should be %t)",
					i, summary.ResEdited, testCases[i].editedRes)
			}
			if summary.HasResponse != testCases[i].hasRes {
				t.Fatalf("TxnSummaries[%d].HasResponse is %t (should be %t)",
					i, summary.HasResponse, testCases[i].hasRes)
			}

		}
	})

	t.Run("GetRequest", func(t *testing.T) {
		for i, tc := range testCases {
			_, err := store.GetRequest(uint64(i), false)
			if err != nil {
				t.Fatalf("could not get request (id=%d): %s", i, err)
			}

			if tc.editedReq {
				_, err := store.GetRequest(uint64(i), true)
				if err != nil {
					t.Fatalf("could not get edited request (id=%d): %s", i, err)
				}
			}
		}
	})

	t.Run("GetResponse", func(t *testing.T) {
		// TODO: check response body
		for i, tc := range testCases {
			if tc.hasRes {
				_, err := store.GetResponse(uint64(i), false)
				if err != nil {
					t.Fatalf("could not get response (id=%d): %s", i, err)
				}

				if tc.editedRes {
					_, err := store.GetResponse(uint64(i), true)
					if err != nil {
						t.Fatalf("could not get edited response (id=%d): %s", i, err)
					}
				}

			}
		}
	})

	t.Run("GetRequest(invalid)", func(t *testing.T) {
		_, err := store.GetRequest(uint64(len(testCases)), false)
		if err != badger.ErrKeyNotFound {
			t.Fatalf("fetching invalid request returned the wrong error (`%s` instead of `%s`)",
				err, badger.ErrKeyNotFound)
		}
	})

	t.Run("GetSummary", func(t *testing.T) {
		for i, tc := range testCases {
			summary, err := store.GetSummary(uint64(i))
			if err != nil {
				t.Fatalf("could not fetch summary with ID %d", i)
			}

			if summary.ID != uint64(i) {
				t.Fatalf("summary%d has wrong ID %[1]d (should be %[2]d)",
					i, summary.ID, i)
			}
			if summary.ReqEdited != tc.editedReq {
				t.Fatalf("summary%d.ReqEdited is %t (should be %t)",
					i, summary.ReqEdited, tc.editedReq)
			}
			if summary.ResEdited != tc.editedRes {
				t.Fatalf("summary%d.ResEdited is %t (should be %t)",
					i, summary.ResEdited, testCases[i].editedRes)
			}
			if summary.HasResponse != tc.hasRes {
				t.Fatalf("summary%d.HasResponse is %t (should be %t)",
					i, summary.HasResponse, testCases[i].hasRes)
			}
		}
	})

	t.Run("GetTxn", func(t *testing.T) {
		for i, tc := range testCases {
			txn, err := store.GetTxn(uint64(i))
			if err != nil {
				t.Fatalf("could not fetch txn with ID %d", i)
			}

			if txn.ID != uint64(i) {
				t.Fatalf("txn%d has wrong ID %[1]d (should be %[2]d)",
					i, txn.ID, i)
			}
			if (txn.ReqE != nil) != tc.editedReq {
				t.Fatalf("txn%d has edited request is %t (should be %t)",
					i, (txn.ReqE != nil), tc.editedReq)
			}
			if (txn.ResE != nil) != tc.editedRes {
				t.Fatalf("txn%d has edited response is %t (should be %t)",
					i, (txn.ResE != nil), testCases[i].editedRes)
			}
			if (txn.Res != nil) != tc.hasRes {
				t.Fatalf("txn%d has response is %t (should be %t)",
					i, (txn.Res != nil), testCases[i].hasRes)
			}
		}
	})

	t.Run("Close", func(t *testing.T) {
		err := store.Close()
		if err != nil {
			t.Fatalf("closing TxnStore failed: %s", err)
		}
	})
}
