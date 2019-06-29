package store

import (
	"fmt"
	"regexp"
	"strconv"
)

// KeyType is used to distinguish between requests and responses
// in store keys.
type KeyType string

// These constants define the key structure of the store.
const (
	// KeyTemplate is the key template to be filled with ID,
	// KeyType and EditedPostfix/OriginalPostfix in that order.
	KeyTemplate             = "%d-%s-%s"
	ReqType         KeyType = "Req"
	ResType         KeyType = "Res"
	EditedPostfix           = "E"
	OriginalPostfix         = "O"
)

// KeyRegex is the regex used to extract info from a key in the KeyTemplate form.
var KeyRegex = regexp.MustCompile(`^(\d+)-(.+)-(.+)$`)

// Key represents the elements that are serialized to the
// actual store key.
type Key struct {
	ID     uint64
	Type   KeyType
	Edited bool
}

// Bytes serializes the Key struct such that it can be used
// as an actual store key.
func (k Key) Bytes() []byte {
	postfix := OriginalPostfix
	if k.Edited {
		postfix = EditedPostfix
	}
	return []byte(fmt.Sprintf(KeyTemplate, k.ID, k.Type, postfix))
}

// ParseKey creates a Key object from the bytes of the actual key.
func ParseKey(storeKey []byte) (key *Key, err error) {
	matches := KeyRegex.FindStringSubmatch(string(storeKey))
	if len(matches) != 4 {
		return nil, fmt.Errorf("could not parse key: %s (%v)", string(storeKey), matches)
	}
	rawID := matches[1]
	rawType := matches[2]
	postfix := matches[3]

	key = &Key{}
	key.ID, err = strconv.ParseUint(rawID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("cannot parse ID from key: %s", rawID)
	}

	keyType := KeyType(rawType)
	if keyType != ReqType && keyType != ResType {
		return nil, fmt.Errorf("invalid key kind: %s", rawType)
	}
	key.Type = keyType

	if postfix == OriginalPostfix {
		key.Edited = false
	} else if postfix == EditedPostfix {
		key.Edited = true
	} else {
		return nil, fmt.Errorf("invalid edited postfix: %s", postfix)
	}

	return key, nil
}
