package eventbus

import (
    "sync"
)

type Topic string

type Message struct {
    Topic Topic
    Data  interface{}
}

type Subscriber chan Message

type Bus struct {
    topics map[Topic][]Subscriber
    rw     sync.RWMutex
}

func New() *Bus {
    return &Bus{
        topics: map[Topic][]Subscriber{},
    }
}

func (bus *Bus) Subscribe(topic Topic, subscriber Subscriber) {
    bus.rw.Lock()
    if subscribers, ok := bus.topics[topic]; ok {
        bus.topics[topic] = append(subscribers, subscriber)
    } else {
        bus.topics[topic] = []Subscriber{subscriber}
    }
    bus.rw.Unlock()
}

func (bus *Bus) Publish(topic Topic, data interface{}) {
    bus.rw.RLock()
    if subscribers, ok := bus.topics[topic]; ok {
        go func(msg Message, subs []Subscriber) {
            for _, subscriber := range subs {
                subscriber <- msg
            }
        }(Message{Topic: topic, Data: data}, append([]Subscriber{}, subscribers...))
    }
    bus.rw.RUnlock()
}

func (bus *Bus) UnSubscribe(topic Topic, subscriber Subscriber) {
    bus.rw.Lock()
    if subs, ok := bus.topics[topic]; ok {
        for i, s := range subs {
            if s == subscriber {
                bus.topics[topic] = append(subs[:i], subs[i+1:]...)
                break
            }
        }
    }
    bus.rw.Unlock()
}
