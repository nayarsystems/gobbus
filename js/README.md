# Gobbus Javascript Wrapper

This is a Javascript wrapper of the gobbus client, using GopherJS

# Usage

Importing this module defines the global object "obbus", which has a connect promise that will get you an obbus connection:
`obbus.connect("wss://x.x.x.x").then((bus) => {...})`

## Obbus connection methods

* `bus.subscribe(topic, callback)`
> Returns a handler to cancel the subscription

* `bus.unsubscribeAll()`
* `bus.publishString(topic, string, flags, rtopic)`
* `bus.publishInteger(topic, int, flags, rtopic)`
* `bus.publishDouble(topic, float, flags, rtopic)`
* `bus.publishBuffer(topic, UInt8Array, flags, rtopic)`
* `bus.rpcString(topic, string, flags, timeoutms)`
* `bus.rpcInteger(topic, int, flags, timeoutms)`
* `bus.rpcDouble(topic, float, flags, timeoutms)`
* `bus.rpcBuffer(topic, UInt8Array, flags, timeoutms)`

# Flags
`obbus.newMessageFlags()` returns a new map with the default flags.

You could also use an empty map, which will become the default all-false flags, or a map only with the flags that you want to set. More info in the following examples.

# Examples

```
obbus.connect("wss://wsgw.n4m.zone/?method=n4m&to=10.100.0.18:2324&user=abc&pass=def")
.then((bus) => {
    subhandler = bus.subscribe("*", console.log)
    setTimeout(subhandler.cancel, 5000)

    flags = obbus.newMessageFlags()
    flags.Instant = true

    bus.publishString("asdf", "abc", flags, "")

    bus.publishString("asdf", "hello", {}, "")
        .then(() => bus.close())
        .catch((err) => console.log(err))
})
```


```
obbus.connect("wss://wsgw.n4m.zone/?method=n4m&to=10.100.0.18:2324&user=abc&pass=def")
.then((bus) => {
    console.log("Connected!")

    bus.rpcInteger("modem.state", 1, {"Instant": true}, 2000)
        .then((response) => {
            console.log("Modem state:", response)
            bus.close()
        })
})
```