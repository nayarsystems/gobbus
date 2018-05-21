package gobbus

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jaracil/ei"
	"gopkg.in/vmihailenco/msgpack.v2"
)

type syncMsg struct {
	cmd     int
	fromCmd int
	val     int
}

type syncCmdSlot struct {
	ch  chan *syncMsg
	ctx context.Context
}

func (o *Obbus) execSyncCmd(vals ...interface{}) (int, error) {
	cmd := ei.N(vals[0]).IntZ()
	msg, err := o.syncCmd(cmd, vals...)
	if err != nil {
		o.Close(err)
		return 0, fmt.Errorf("Failed command (%d: %s): %s", cmd, commandToStr(cmd), err)
	}
	if msg.cmd == cmdError {
		return 0, fmt.Errorf("Failed command (%d: %s): obbus error (%d: %s)", cmd, commandToStr(cmd), msg.val, errToStr(msg.val))
	}
	return msg.val, nil
}

func (o *Obbus) syncCmd(cmd int, vals ...interface{}) (msg *syncMsg, err error) {
	var ctx context.Context
	var cancel context.CancelFunc
	var slot *syncCmdSlot

	if cmd != cmdPublish && cmd != cmdClose {
		ctx, cancel = context.WithTimeout(o.ctx, o.SyncCmdTimeout)
		defer cancel()
		if slot, err = o.reserveSlot(cmd, ctx); err != nil {
			return
		}
	}

	o.writeLock.Lock()
	err = o.writeSyncCmd(cmd, vals...)
	if err != nil {
		o.writeLock.Unlock()
		if o.closed {
			err = fmt.Errorf("obbus is closed")
		} else {
			o.Close(err)
		}
		return
	}
	o.keepAliveTimer.Reset(o.timeout)
	o.writeLock.Unlock()

	if o.Debug {
		log.Printf("%s>> %v\n", o.DebugLabel, vals)
	}

	if cmd == cmdPublish || cmd == cmdClose {
		msg = &syncMsg{
			cmd:     cmdResponse,
			fromCmd: cmd,
			val:     1,
		}
		return
	}

	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("timeout waiting response")
		} else {
			err = fmt.Errorf("obbus is closed")
		}
		return
	case msg = <-slot.ch:
		return
	}
}

func (o *Obbus) reserveSlot(cmd int, ctx context.Context) (*syncCmdSlot, error) {
	for {
		o.syncLock.Lock()
		if current, ok := o.syncSlots[cmd]; !ok {
			slot := &syncCmdSlot{
				ch:  make(chan *syncMsg, 1),
				ctx: ctx,
			}
			o.syncSlots[cmd] = slot
			o.syncLock.Unlock()
			return slot, nil
		} else {
			o.syncLock.Unlock()
			select {
			case <-current.ctx.Done():
				continue
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					return nil, fmt.Errorf("timeout waiting slot to send")
				} else {
					return nil, fmt.Errorf("obbus is closed")
				}
			}
		}
	}
}

func (o *Obbus) writeResponseAndFreeSlot(msg *syncMsg) {
	o.syncLock.Lock()
	if slot, ok := o.syncSlots[msg.fromCmd]; ok {
		select {
		case slot.ch <- msg:
		default:
		}
		delete(o.syncSlots, msg.fromCmd)
	}
	o.syncLock.Unlock()
}

func (o *Obbus) writeSyncCmd(cmd int, vals ...interface{}) (err error) {
	var buf bytes.Buffer

	enc := msgpack.NewEncoder(&buf)

	if err = enc.EncodeArrayLen(len(vals)); err != nil {
		return
	}
	if err = enc.EncodeInt(cmd); err != nil {
		return
	}

	switch cmd {
	case cmdPing:
		if err = enc.EncodeInt(ei.N(vals[1]).IntZ()); err != nil {
			return
		}
	case cmdTopicTableCreate:
		if err = enc.EncodeInt(ei.N(vals[1]).IntZ()); err != nil {
			return
		}
	case cmdTopicTableSet:
		if err = enc.EncodeInt(ei.N(vals[1]).IntZ()); err != nil {
			return
		}
		if err = enc.EncodeString(ei.N(vals[2]).StringZ()); err != nil {
			return
		}
	case cmdTimeout:
		if err = enc.EncodeInt(ei.N(vals[1]).IntZ()); err != nil {
			return
		}
	case cmdSubscribe:
		sub := ei.N(vals[1]).StringZ()
		if err = enc.EncodeString(sub); err != nil {
			return
		}
	case cmdUnsubscribe:
		unsub := ei.N(vals[1]).StringZ()
		if err = enc.EncodeString(unsub); err != nil {
			return
		}
	case cmdUnsubscribeAll:
	case cmdPublishAck:
		fallthrough
	case cmdPublish:
		topic := ei.N(vals[1]).StringZ()
		idx, ok := o.getTopicIndex(topic)
		if ok {
			if err = enc.EncodeInt(idx); err != nil {
				return
			}
		} else {
			if err = enc.EncodeString(topic); err != nil {
				return
			}
		}
		if intVal, ok := vals[2].(int); ok {
			if err = enc.EncodeInt(intVal); err != nil {
				return
			}
		} else if strVal, ok := vals[2].(string); ok {
			if err = enc.EncodeString(strVal); err != nil {
				return
			}
		} else if dblVal, ok := vals[2].(float64); ok {
			if err = enc.EncodeFloat64(dblVal); err != nil {
				return
			}
		} else if binVal, ok := vals[2].([]byte); ok {
			if err = enc.EncodeBytes(binVal); err != nil {
				return
			}
		}
		if len(vals) > 3 {
			flags := ei.N(vals[3]).UintZ()
			if err = enc.EncodeUint(flags); err != nil {
				return
			}
		}
		if len(vals) > 4 {
			rtopic := ei.N(vals[4]).StringZ()
			ridx, ok := o.getTopicIndex(rtopic)
			if ok {
				if err = enc.EncodeInt(ridx); err != nil {
					return
				}
			} else {
				if err = enc.EncodeString(rtopic); err != nil {
					return
				}
			}
		}
	case cmdClose:
	}

	if o.conn != nil {
		_, err = o.conn.Write(buf.Bytes())
	} else {
		err = fmt.Errorf("obbus is closed")
	}

	return
}

func (o *Obbus) Ping(n int) (int, error) {
	res, err := o.execSyncCmd(cmdPing, n)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (o *Obbus) SetTimeout(d time.Duration) error {
	_, err := o.execSyncCmd(cmdTimeout, int(d.Seconds()))
	if err != nil {
		return err
	}
	o.timeout = d / 2
	o.Ping(0)
	return nil
}

func (o *Obbus) Subscribe(ch chan *Message, path string) error {
	if o.addAsyncSub(path, ch) {
		_, err := o.execSyncCmd(cmdSubscribe, path)
		if err != nil {
			o.delAsyncSub(path, ch)
			return err
		}
	}
	return nil
}

func (o *Obbus) Unsubscribe(ch chan *Message, path string) error {
	if o.delAsyncSub(path, ch) {
		_, err := o.execSyncCmd(cmdUnsubscribe, path)
		if err != nil {
			o.addAsyncSub(path, ch)
			return err
		}
	}
	return nil
}

func (o *Obbus) UnsubscribeAll() (int, error) {
	res, err := o.execSyncCmd(cmdUnsubscribeAll)
	if err != nil {
		return 0, err
	}
	o.subscribeRpc()
	o.delAllAsyncSubs()
	return res, nil
}

func (o *Obbus) PublishInt(topic string, val int, flags *MessageFlags, rtopic string, ack bool) (int, error) {
	return o.publish(topic, val, flagsToUint(flags), rtopic, ack)
}
func (o *Obbus) PublishStr(topic string, val string, flags *MessageFlags, rtopic string, ack bool) (int, error) {
	return o.publish(topic, val, flagsToUint(flags), rtopic, ack)
}
func (o *Obbus) PublishDbl(topic string, val float64, flags *MessageFlags, rtopic string, ack bool) (int, error) {
	return o.publish(topic, val, flagsToUint(flags), rtopic, ack)
}
func (o *Obbus) PublishBuf(topic string, val []byte, flags *MessageFlags, rtopic string, ack bool) (int, error) {
	return o.publish(topic, val, flagsToUint(flags), rtopic, ack)
}

func (o *Obbus) publish(topic string, val interface{}, flags uint, rtopic string, ack bool) (int, error) {
	var args []interface{}
	if ack {
		args = []interface{}{cmdPublishAck, topic, val}
	} else {
		args = []interface{}{cmdPublish, topic, val}
	}
	if rtopic != "" {
		args = append(args, flags, rtopic)
	} else if flags != 0 {
		args = append(args, flags)
	}
	res, err := o.execSyncCmd(args...)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (o *Obbus) SetTopicTable(table []string) error {
	if table == nil {
		table = []string{}
	}
	res, err := o.execSyncCmd(cmdTopicTableCreate, len(table))
	if err != nil {
		return err
	}
	if res != 0 {
		return fmt.Errorf("Failed command (%d: %s): Requested topic table size excedes maximum allowed size", cmdTopicTableCreate, commandToStr(cmdTopicTableCreate))
	}
	o.topicTableReset()
	for i, v := range table {
		o.topicTableSet(i, v)
		res, err := o.execSyncCmd(cmdTopicTableSet, i, v)
		if err != nil {
			o.topicTableReset()
			return err
		}
		if res != 0 {
			o.topicTableReset()
			return fmt.Errorf("Failed command (%d: %s): Requested invalid topic index", cmdTopicTableCreate, commandToStr(cmdTopicTableCreate))
		}
	}
	return nil
}

func (o *Obbus) topicTableReset() {
	o.topicTableLock.Lock()
	o.topicToIndex = map[string]int{}
	o.indexToTopic = map[int]string{}
	o.topicTableLock.Unlock()
}

func (o *Obbus) topicTableSet(i int, v string) {
	o.topicTableLock.Lock()
	o.topicToIndex[v] = i
	o.indexToTopic[i] = v
	o.topicTableLock.Unlock()
}

func (o *Obbus) getTopicIndex(v string) (int, bool) {
	o.topicTableLock.Lock()
	i, ok := o.topicToIndex[v]
	o.topicTableLock.Unlock()
	return i, ok
}

func (o *Obbus) getIndexTopic(i int) (string, bool) {
	o.topicTableLock.Lock()
	v, ok := o.indexToTopic[i]
	o.topicTableLock.Unlock()
	return v, ok
}
