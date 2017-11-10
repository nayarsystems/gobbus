package gobbus

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

func (o *Obbus) newRtopic() string {
	return fmt.Sprintf("%s%d", o.rpcPrefix, atomic.AddUint64(&o.rpcN, 1))
}

func (o *Obbus) addRpc(rtopic string) chan *Message {
	o.rpcLock.Lock()
	ch := make(chan *Message, 1)
	o.rpcs[rtopic] = ch
	o.rpcLock.Unlock()
	return ch
}

func (o *Obbus) delRpc(rtopic string) {
	o.rpcLock.Lock()
	if rpc, ok := o.rpcs[rtopic]; ok {
		close(rpc)
		delete(o.rpcs, rtopic)
	}
	o.rpcLock.Unlock()
}

func (o *Obbus) rpcMsg(m *Message) {
	o.rpcLock.Lock()
	if ch, ok := o.rpcs[m.Topic]; ok {
		select {
		case ch <- m:
		default:
		}
	}
	o.rpcLock.Unlock()
}

func (o *Obbus) RpcInt(path string, val int, flags *MessageFlags, tout time.Duration) (*Message, error) {
	return o.rpc(path, val, flagsToUint(flags), tout)
}

func (o *Obbus) RpcStr(path string, val string, flags *MessageFlags, tout time.Duration) (*Message, error) {
	return o.rpc(path, val, flagsToUint(flags), tout)
}

func (o *Obbus) RpcDbl(path string, val float64, flags *MessageFlags, tout time.Duration) (*Message, error) {
	return o.rpc(path, val, flagsToUint(flags), tout)
}

func (o *Obbus) RpcBuf(path string, val []byte, flags *MessageFlags, tout time.Duration) (*Message, error) {
	return o.rpc(path, val, flagsToUint(flags), tout)
}

func (o *Obbus) rpc(path string, val interface{}, flags uint, tout time.Duration) (*Message, error) {
	rtopic := o.newRtopic()
	ch := o.addRpc(rtopic)
	_, err := o.publish(path, val, flags, rtopic, false)
	if err != nil {
		return nil, fmt.Errorf("Failed rpc (%s): %s", path, err.Error())
	}
	ctx, cancel := context.WithTimeout(o.ctx, tout)
	select {
	case <-ctx.Done():
		o.delRpc(rtopic)
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("Failed rpc (%s): timeout waiting response", path)
		} else {
			return nil, fmt.Errorf("Failed rpc (%s): obbus is closed", path)
		}
	case msg := <-ch:
		o.delRpc(rtopic)
		cancel()
		return msg, nil
	}
}
