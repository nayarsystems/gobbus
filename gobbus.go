package gobbus

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"sync"

	"github.com/jaracil/ei"
	"gopkg.in/vmihailenco/msgpack.v2"
)

// Protocol commands
const (
	cmdPing             = 0x00
	cmdSubscribe        = 0x01
	cmdUnsubscribe      = 0x02
	cmdUnsubscribeAll   = 0x03
	cmdPublish          = 0x04
	cmdPublishAck       = 0x05
	cmdTimeout          = 0x06
	cmdClose            = 0x07
	cmdTopicTableCreate = 0x08
	cmdTopicTableSet    = 0x09
	cmdMessage          = 0x10
	cmdResponse         = 0x11
	cmdError            = 0x12
)

var _commandStr = map[int]string{
	cmdPing:             "ping",
	cmdSubscribe:        "subscribe",
	cmdUnsubscribe:      "unsubscribe",
	cmdUnsubscribeAll:   "unsubscribeAll",
	cmdPublish:          "publish",
	cmdPublishAck:       "publishAck",
	cmdTimeout:          "timeout",
	cmdClose:            "close",
	cmdTopicTableCreate: "topicTableCreate",
	cmdTopicTableSet:    "topicTableSet",
	cmdMessage:          "message",
	cmdResponse:         "response",
	cmdError:            "error",
}

func commandToStr(p int) string {
	return _commandStr[p]
}

// Protocol errors
const (
	errInvalidCommand    = 0x01
	errInvalidParamNum   = 0x02
	errInvalidParamType  = 0x03
	errInvalidFrame      = 0x04
	errUnauthorized      = 0x05
	errInvalidTopicIndex = 0x06
)

var _errStr = map[int]string{
	errInvalidCommand:    "invalid command",
	errInvalidParamNum:   "invalid number of parameters",
	errInvalidParamType:  "invalid type of parameter",
	errInvalidFrame:      "invalid frame",
	errUnauthorized:      "unauthorized",
	errInvalidTopicIndex: "invalid topic index",
}

func errToStr(e int) string {
	return _errStr[e]
}

// Obbus
type Obbus struct {
	conn              io.ReadWriteCloser
	ctx               context.Context
	cancel            context.CancelFunc
	decoder           *msgpack.Decoder
	closed            bool
	err               error
	syncSlots         map[int]*syncCmdSlot
	syncLock          *sync.Mutex
	subscriptions     map[string][]chan *Message
	subscriptionsLock *sync.Mutex
	topicToIndex      map[string]int
	indexToTopic      map[int]string
	topicTableLock    *sync.Mutex
	rpcPrefix         string
	rpcN              uint64
	rpcLock           *sync.Mutex
	rpcs              map[string]chan *Message
	writeLock         *sync.Mutex
	timeout           time.Duration
	keepAliveTimer    *time.Timer
	SyncCmdTimeout    time.Duration
	Debug             bool
	DebugLabel        string
}

type ObbusOpts struct {
	Timeout        time.Duration
	SyncCmdTimeout time.Duration
	Debug          bool
	DebugLabel     string
}

func NewObbus(conn io.ReadWriteCloser, opts ...*ObbusOpts) *Obbus {
	tout := time.Second * 300
	cmdTout := time.Second * 6
	debug := false
	label := ""
	if len(opts) != 0 {
		if opts[0].Timeout != 0 {
			tout = opts[0].Timeout
		}
		if opts[0].SyncCmdTimeout != 0 {
			cmdTout = opts[0].SyncCmdTimeout
		}
		if opts[0].Debug {
			debug = true
		}
		if opts[0].DebugLabel != "" {
			label = opts[0].DebugLabel + " "
		}
	}

	obCtx, obCancel := context.WithCancel(context.Background())

	obbus := &Obbus{
		conn:              conn,
		ctx:               obCtx,
		cancel:            obCancel,
		decoder:           msgpack.NewDecoder(conn),
		err:               nil,
		closed:            false,
		syncSlots:         map[int]*syncCmdSlot{},
		syncLock:          &sync.Mutex{},
		subscriptions:     map[string][]chan *Message{},
		subscriptionsLock: &sync.Mutex{},
		topicToIndex:      map[string]int{},
		indexToTopic:      map[int]string{},
		topicTableLock:    &sync.Mutex{},
		rpcPrefix:         fmt.Sprintf("rpc%s.", genRandString(6)),
		rpcN:              0,
		rpcLock:           &sync.Mutex{},
		rpcs:              map[string]chan *Message{},
		writeLock:         &sync.Mutex{},
		timeout:           tout,
		keepAliveTimer:    time.NewTimer(tout),
		SyncCmdTimeout:    cmdTout,
		Debug:             debug,
		DebugLabel:        label,
	}

	go obbus.recvWorker()
	go obbus.keepAlive()

	obbus.subscribeRpc()

	return obbus
}

func (o *Obbus) subscribeRpc() {
	_, err := o.execSyncCmd(cmdSubscribe, fmt.Sprintf("%s*", o.rpcPrefix))
	if err != nil {
		o.Close(err)
	}
}

func (o *Obbus) Close(err error) {
	if err != nil && o.err == nil {
		o.err = err
		if o.Debug {
			log.Printf("%s!! %s\n", o.DebugLabel, err.Error())
		}
	}
	o.syncLock.Lock()
	if o.conn != nil {
		o.cancel()
		o.closed = true
		o.syncCmd(cmdClose, cmdClose)
		o.conn.Close()
		o.conn = nil
	}
	o.syncSlots = map[int]*syncCmdSlot{}
	o.syncLock.Unlock()
	o.topicTableLock.Lock()
	o.topicToIndex = map[string]int{}
	o.indexToTopic = map[int]string{}
	o.topicTableLock.Unlock()
	o.subscriptionsLock.Lock()
	closed := map[chan *Message]bool{}
	for _, subs := range o.subscriptions {
		for _, ch := range subs {
			if _, ok := closed[ch]; !ok {
				closed[ch] = true
				close(ch)
			}
		}
	}
	o.subscriptions = map[string][]chan *Message{}
	o.subscriptionsLock.Unlock()
}

func (o *Obbus) GetContext() context.Context {
	return o.ctx
}

func (o *Obbus) keepAlive() {
	ctx, _ := context.WithCancel(o.ctx)
	for {
		select {
		case <-o.keepAliveTimer.C:
			o.Ping(0)
		case <-ctx.Done():
			return
		}
	}
}

func (o *Obbus) recvWorker() {
	for {
		if o.closed {
			break
		}
		recv, err := o.decoder.DecodeSlice()
		if err != nil {
			if err == io.EOF {
				o.Close(fmt.Errorf("Connection was closed: EOF"))
			} else {
				o.Close(fmt.Errorf("Error decoding array: %s", err.Error()))
			}
			break
		}
		if o.closed {
			break
		}
		if o.Debug {
			log.Printf("%s<< %v\n", o.DebugLabel, recv)
		}
		cmd := ei.N(recv[0]).IntZ()
		if cmd == cmdMessage {
			o.handleMessage(recv)
		} else {
			o.handleResErr(cmd, recv)
		}
	}
}

func (o *Obbus) handleResErr(cmd int, recv []interface{}) {
	o.writeResponseAndFreeSlot(&syncMsg{cmd: cmd, fromCmd: ei.N(recv[1]).IntZ(), val: ei.N(recv[2]).IntZ()})
}

func (o *Obbus) handleMessage(recv []interface{}) {
	var flags uint
	var topic, rtopic string
	var ok bool
	flags = ei.N(recv[1]).UintZ()
	topic, ok = recv[2].(string)
	if !ok {
		topic, _ = o.getIndexTopic(ei.N(recv[2]).IntZ())
	}
	if len(recv) > 4 {
		rtopic = ei.N(recv[4]).StringZ()
	}
	v := recv[3]
	switch val := v.(type) {
	case int64:
		v = int(val)
	case uint64:
		v = int(val)
	}
	if strings.HasPrefix(topic, o.rpcPrefix) {
		o.rpcMsg(&Message{topic, flagsToStruct(flags), v, rtopic})
	} else {
		o.asyncMsg(&Message{topic, flagsToStruct(flags), v, rtopic})
	}
}
