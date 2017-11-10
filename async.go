package gobbus

import (
	"fmt"
	"strings"
)

type Message struct {
	Topic  string
	Flags  *MessageFlags
	val    interface{}
	Rtopic string
}

type MessageFlags struct {
	Instant      bool
	NonRecursive bool
	Response     bool
	Error        bool
}

const (
	msgFlInstant      = 1 << 0
	msgFlNonrecursive = 1 << 1
	msgFlResponse     = 1 << 2
	msgFlError        = 1 << 3
	msgFlMask         = 0xff
)

func flagsToUint(f *MessageFlags) uint {
	var flags uint
	if f != nil {
		if f.Instant {
			flags |= msgFlInstant
		}
		if f.NonRecursive {
			flags |= msgFlNonrecursive
		}
		if f.Response {
			flags |= msgFlResponse
		}
		if f.Error {
			flags |= msgFlError
		}
	}
	return flags
}

func flagsToStruct(f uint) *MessageFlags {
	flags := &MessageFlags{}
	if (f & msgFlInstant) != 0 {
		flags.Instant = true
	}
	if (f & msgFlNonrecursive) != 0 {
		flags.NonRecursive = true
	}
	if (f & msgFlResponse) != 0 {
		flags.Response = true
	}
	if (f & msgFlError) != 0 {
		flags.Error = true
	}
	return flags
}

func getTopicPaths(path string) []string {
	ps := []string{"*", path}
	spl := strings.Split(path, ".")
	for i := 1; i <= len(spl); i++ {
		ps = append(ps, strings.Join(spl[:i], ".")+".*")
	}
	return ps
}

func (o *Obbus) asyncMsg(m *Message) {
	o.subscriptionsLock.Lock()
	for _, p := range getTopicPaths(m.Topic) {
		if chs, ok := o.subscriptions[p]; ok {
			for _, ch := range chs {
				select {
				case ch <- m:
				default:
				}
			}
		}
	}
	o.subscriptionsLock.Unlock()
}

func (o *Obbus) addAsyncSub(s string, addch chan *Message) bool {
	o.subscriptionsLock.Lock()
	chs, ok := o.subscriptions[s]
	if !ok {
		chs = []chan *Message{}
	}
	found := false
	for _, ch := range chs {
		if ch == addch {
			found = true
			break
		}
	}
	if !found {
		chs = append(chs, addch)
	}
	o.subscriptions[s] = chs
	o.subscriptionsLock.Unlock()
	return !ok
}

func (o *Obbus) delAsyncSub(s string, delch chan *Message) bool {
	var delSub bool
	o.subscriptionsLock.Lock()
	chs, ok := o.subscriptions[s]
	if ok {
		for i, ch := range chs {
			if ch == delch {
				close(ch)
				o.subscriptions[s] = append(chs[:i], chs[i+1:]...)
			}
		}
		if delSub = len(o.subscriptions[s]) == 0; delSub {
			delete(o.subscriptions, s)
		}
	}
	o.subscriptionsLock.Unlock()
	return delSub
}

func (o *Obbus) delAllAsyncSubs() {
	o.subscriptionsLock.Lock()
	for _, chs := range o.subscriptions {
		for _, ch := range chs {
			close(ch)
		}
	}
	o.subscriptions = map[string][]chan *Message{}
	o.subscriptionsLock.Unlock()
}

func (m *Message) String() string {
	r := fmt.Sprintf("topic (%s) ", m.Topic)
	switch val := m.val.(type) {
	case int:
		r += fmt.Sprintf("val (int: %d)", val)
	case string:
		r += fmt.Sprintf("val (string: %s)", val)
	case float64:
		r += fmt.Sprintf("val (float64: %f)", val)
	case []byte:
		r += fmt.Sprintf("val (buf: %x)", val)
	default:
		r += fmt.Sprintf("val (invalid %T: %v)", val, val)
	}
	r += fmt.Sprintf(" flags (%s) rtopic (%s)", m.Flags, m.Rtopic)
	return r
}

func (f *MessageFlags) String() string {
	rs := []string{}
	if f.Instant {
		rs = append(rs, "instant")
	}
	if f.NonRecursive {
		rs = append(rs, "non-recursive")
	}
	if f.Response {
		rs = append(rs, "response")
	}
	if f.Error {
		rs = append(rs, "error")
	}
	return strings.Join(rs, " ")
}

func (m *Message) GetValue() interface{} {
	return m.val
}

func (m *Message) GetInt() (int, error) {
	i, ok := m.val.(int)
	if ok {
		return i, nil
	}
	return 0, fmt.Errorf("message value is not int")
}

func (m *Message) GetStr() (string, error) {
	s, ok := m.val.(string)
	if ok {
		return s, nil
	}
	return "", fmt.Errorf("message value is not string")
}

func (m *Message) GetDbl() (float64, error) {
	f, ok := m.val.(float64)
	if ok {
		return f, nil
	}
	return 0, fmt.Errorf("message value is not double")
}

func (m *Message) GetBuf() ([]byte, error) {
	b, ok := m.val.([]byte)
	if ok {
		return b, nil
	}
	return nil, fmt.Errorf("message value is not buffer")
}
