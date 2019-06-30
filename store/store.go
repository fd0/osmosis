package store

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"

	"github.com/dgraph-io/badger"
)

// Txn represents a transaction consisting of a request
// and response as well as their edited counterparts.
type Txn struct {
	ID   uint64
	Req  *http.Request
	ReqE *http.Request
	Res  *http.Response
	ResE *http.Response
}

// TxnSummary summarizes a Transaction, such a summary can then
// be helt in memory (e.g. for a transaction history) and the
// included ID can then be used to fetch the full content.
type TxnSummary struct {
	ID          uint64
	Host        string
	Method      string
	StatusCode  int
	URL         *url.URL
	ReqEdited   bool
	ResEdited   bool
	HasResponse bool
}

// TxnStore is a key value store mapping
// IDs to request/response-transactions.
type TxnStore struct {
	*badger.DB

	OnUpdate func(uint64)
}

// NewTxnStore returns a pointer to a new TxnStore.
func NewTxnStore(storeDir string) (*TxnStore, error) {
	opts := badger.DefaultOptions
	opts.Dir = storeDir
	opts.ValueDir = storeDir
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &TxnStore{DB: db}, nil
}

// Close closes the underlying database gracefully.
func (s *TxnStore) Close() error {
	return s.DB.Close()
}

// AddRequest adds a new request to the store and triggers an OnUpdate event.
func (s *TxnStore) AddRequest(id uint64, req *http.Request, edited bool) error {
	var reqDump bytes.Buffer
	err := req.WriteProxy(&reqDump)
	if err != nil {
		return err
	}
	err = s.Update(func(txn *badger.Txn) error {
		// TODO: what if the key already exists?
		return txn.Set(Key{ID: id, Type: ReqType, Edited: edited}.Bytes(), reqDump.Bytes())
	})
	if err != nil {
		return err
	}
	if s.OnUpdate != nil {
		s.OnUpdate(id)
	}
	return nil
}

// AddResponse adds a new response to the store and triggers an OnUpdate event.
func (s *TxnStore) AddResponse(id uint64, res *http.Response, body []byte, edited bool) error {
	// Body is already read and closed, we will add it later
	resDump, err := httputil.DumpResponse(res, false)
	if err != nil {
		return err
	}
	resDump = append(resDump, body...)

	err = s.Update(func(txn *badger.Txn) error {
		// TODO: what if the key already exists
		return txn.Set(Key{ID: id, Type: ResType, Edited: edited}.Bytes(), resDump)
	})
	if err != nil {
		return err
	}
	if s.OnUpdate != nil {
		s.OnUpdate(id)
	}
	return nil
}

// GetRequest fetches the original or edited request with the specified ID from the store.
func (s *TxnStore) GetRequest(id uint64, edited bool) (request *http.Request, e error) {
	err := s.View(func(txn *badger.Txn) error {
		item, err := txn.Get(Key{ID: id, Type: ReqType, Edited: edited}.Bytes())
		if err != nil {
			return err
		}
		request, err = parseRequest(item)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return request, nil
}

// GetResponse fetches the original or edited response with the specified ID from the store.
func (s *TxnStore) GetResponse(id uint64, edited bool) (response *http.Response, e error) {
	err := s.View(func(txn *badger.Txn) error {
		item, err := txn.Get(Key{ID: id, Type: ResType, Edited: edited}.Bytes())
		if err != nil {
			return err
		}
		response, err = parseResponse(item)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetSummary returns the TxnSummary for the given ID.
func (s *TxnStore) GetSummary(id uint64) (*TxnSummary, error) {
	summary := &TxnSummary{ID: id}

	req, err := s.GetRequest(id, false)
	if err != nil {
		return nil, err
	}

	summary.Host = req.Host
	summary.Method = req.Method
	summary.URL = req.URL

	req, err = s.GetRequest(id, true)
	if err == nil {
		summary.ReqEdited = true
		summary.Host = req.Host
		summary.Method = req.Method
		summary.URL = req.URL
	} else if err != badger.ErrKeyNotFound {
		return nil, err
	}

	res, err := s.GetResponse(id, false)
	if err == nil {
		summary.HasResponse = true
		summary.StatusCode = res.StatusCode
	} else if err != badger.ErrKeyNotFound {
		return nil, err
	}

	res, err = s.GetResponse(id, true)
	if err == nil {
		summary.HasResponse = true
		summary.ResEdited = true
		summary.StatusCode = res.StatusCode
	} else if err != badger.ErrKeyNotFound {
		return nil, err
	}

	return summary, nil
}

// GetTxn returns the transaction for the given ID.
func (s *TxnStore) GetTxn(id uint64) (*Txn, error) {
	req, err := s.GetRequest(id, false)
	if err != nil {
		return nil, err
	}
	reqe, err := s.GetRequest(id, true)
	if err != nil && err != badger.ErrKeyNotFound {
		return nil, err
	}
	res, err := s.GetResponse(id, false)
	if err != nil && err != badger.ErrKeyNotFound {
		return nil, err
	}
	rese, err := s.GetResponse(id, true)
	if err != nil && err != badger.ErrKeyNotFound {
		return nil, err
	}
	return &Txn{
		ID:   id,
		Req:  req,
		ReqE: reqe,
		Res:  res,
		ResE: rese,
	}, nil
}

// MaxID returns the highest ID stored.
func (s *TxnStore) MaxID() (max uint64, e error) {
	err := s.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		// no prefetch need for key only iteration
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			key, err := ParseKey(it.Item().Key())
			if err != nil {
				return err
			}
			if key.ID > max {
				max = key.ID
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return max, nil
}

// TxnSummaries returns TxnSummaries for all items in the databse.
func (s *TxnStore) TxnSummaries() ([]*TxnSummary, error) {
	summaryMap := make(map[uint64]*TxnSummary)

	err := s.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key, err := ParseKey(item.Key())
			if err != nil {
				return err
			}

			_, ok := summaryMap[key.ID]
			if !ok {
				summaryMap[key.ID] = &TxnSummary{ID: key.ID}
			}
			summary := summaryMap[key.ID]

			switch key.Type {
			case ReqType: // request
				req, err := parseRequest(item)
				if err != nil {
					return fmt.Errorf("yyyy: %s\n%s", err, item)
				}

				if key.Edited {
					summary.ReqEdited = true
				}
				// only update summary if the fields were not overwritten
				// by the edited request in reqe
				if key.Edited || summary.Host == "" {
					summary.Host = req.Host
				}
				if key.Edited || summary.Method == "" {
					summary.Method = req.Method
				}
				if key.Edited || summary.URL == nil {
					summary.URL = req.URL
				}
			case ResType: // response
				res, err := parseResponse(item)
				if err != nil {
					return fmt.Errorf("xxxx: %s\n%s", err, item)
				}

				summary.HasResponse = true
				if key.Edited {
					summary.ResEdited = true
				}
				// only update summary if StatusCode was not overwritten
				// by the edited response in rese
				if key.Edited || summary.StatusCode == 0 {
					summary.StatusCode = res.StatusCode
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	summaries := make([]*TxnSummary, 0, len(summaryMap))
	for k := range summaryMap {
		summaries = append(summaries, summaryMap[k])
	}

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}
