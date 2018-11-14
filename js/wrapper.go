package main

import (
	"context"
	"time"

	"github.com/gopherjs/gopherjs/js"
	"github.com/goxjs/websocket"
	"github.com/nayarsystems/gobbus"
)

func main() {
	js.Global.Set("obbus", map[string]interface{}{
		"connect": func(address string) *js.Object {
			promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
				go func() {
					var bus *gobbus.Obbus

					ret := js.MakeWrapper(map[string]interface{}{})
					connected := false
					subscriptions := make(map[*context.CancelFunc]bool, 0)

					conn, err := websocket.Dial(address, "http://gobbus.jsclient")
					if err != nil {
						rej("Error connecting to websocket gateway: " + err.Error())
						return
					}

					bus = gobbus.NewObbus(conn, &gobbus.ObbusOpts{
						Timeout: time.Second * 30,
					})

					connected = true

					ret.Set("debug", func(d bool) {
						if bus != nil {
							bus.Debug = d
						}
					})

					ret.Set("subscribe", func(path string, cb func(interface{})) *js.Object {
						ctx, cancelFun := context.WithCancel(bus.GetContext())
						subscriptions[&cancelFun] = true

						go func() {
							defer cancelFun()

							if !connected {
								return
							}

							rchan := make(chan *gobbus.Message, 1000)
							bus.Subscribe(rchan, path)

						loop:
							for {
								select {
								case <-ctx.Done():
									break loop

								case msg, ok := <-rchan:
									if !ok {
										break loop
									}

									cb(map[string]interface{}{
										"Topic":         msg.Topic,
										"Value":         msg.GetValue(),
										"Flags":         msg.Flags,
										"ResponseTopic": msg.Rtopic,
									})
								}
							}

							bus.Unsubscribe(rchan, path)
						}()

						ret := js.MakeWrapper(map[string]interface{}{})
						ret.Set("topic", path)
						ret.Set("cancel", func() {
							delete(subscriptions, &cancelFun)
							cancelFun()
						})
						return ret
					})

					ret.Set("unsubscribeAll", func(path string, cb func(interface{})) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								for cancelFun, _ := range subscriptions {
									delete(subscriptions, cancelFun)
									(*cancelFun)()
								}
								res(nil)
							}()
						})
						return promise
					})

					ret.Set("publishString", func(topic string, val string, flags *js.Object, rtopic string) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								i, err := bus.PublishStr(topic, val, parseFlags(flags), rtopic, true)
								if err != nil {
									rej(err.Error())
								} else {
									res(i)
								}
							}()
						})
						return promise
					})

					ret.Set("publishInteger", func(topic string, val int, flags *js.Object, rtopic string) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								i, err := bus.PublishInt(topic, val, parseFlags(flags), rtopic, true)
								if err != nil {
									rej(err.Error())
								} else {
									res(i)
								}
							}()
						})
						return promise
					})

					ret.Set("publishDouble", func(topic string, val float64, flags *js.Object, rtopic string) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								i, err := bus.PublishDbl(topic, val, parseFlags(flags), rtopic, true)
								if err != nil {
									rej(err.Error())
								} else {
									res(i)
								}
							}()
						})
						return promise
					})

					ret.Set("publishBuffer", func(topic string, val []byte, flags *js.Object, rtopic string) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								i, err := bus.PublishBuf(topic, val, parseFlags(flags), rtopic, true)
								if err != nil {
									rej(err.Error())
								} else {
									res(i)
								}
							}()
						})
						return promise
					})

					ret.Set("rpcString", func(topic string, val string, flags *js.Object, timeoutms int64) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								i, err := bus.RpcStr(topic, val, parseFlags(flags), time.Duration(timeoutms*1000))
								if err != nil {
									rej(err.Error())
								} else {
									res(i.GetValue())
								}
							}()
						})
						return promise
					})

					ret.Set("rpcInteger", func(topic string, val int, flags *js.Object, timeoutms int) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								i, err := bus.RpcInt(topic, val, parseFlags(flags), time.Millisecond*time.Duration(timeoutms*1000))
								if err != nil {
									rej(err.Error())
								} else {
									res(i.GetValue())
								}
							}()
						})
						return promise
					})

					ret.Set("rpcDouble", func(topic string, val float64, flags *js.Object, timeoutms int) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								i, err := bus.RpcDbl(topic, val, parseFlags(flags), time.Millisecond*time.Duration(timeoutms*1000))
								if err != nil {
									rej(err.Error())
								} else {
									res(i.GetValue())
								}
							}()
						})
						return promise
					})

					ret.Set("rpcBuffer", func(topic string, val []byte, flags *js.Object, timeoutms int) *js.Object {
						promise := js.Global.Get("Promise").New(func(res, rej func(interface{})) {
							go func() {
								if !connected {
									rej("Not Connected")
									return
								}

								i, err := bus.RpcBuf(topic, val, parseFlags(flags), time.Millisecond*time.Duration(timeoutms*1000))
								if err != nil {
									rej(err.Error())
								} else {
									res(i.GetValue())
								}
							}()
						})
						return promise
					})

					ret.Set("close", func() {
						go func() {
							if bus != nil && bus.GetContext().Err() == nil {
								bus.Close(nil)
							}
						}()
					})

					res(ret)
				}()
			})

			return promise
		},

		"newMessageFlags": func() *gobbus.MessageFlags {
			return &gobbus.MessageFlags{}
		},
	})
}

type MessageFlags struct {
	*js.Object
	Instant      bool `js:"Instant"`
	NonRecursive bool `js:"NonRecursive"`
	Response     bool `js:"Response"`
	Error        bool `js:"Error"`
}

func parseFlags(flags *js.Object) *gobbus.MessageFlags {
	jsflags := &MessageFlags{Object: flags}
	return &gobbus.MessageFlags{
		Instant:      jsflags.Instant,
		Error:        jsflags.Error,
		NonRecursive: jsflags.NonRecursive,
		Response:     jsflags.Response,
	}
}
