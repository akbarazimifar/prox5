package prox5

import (
	"errors"
	"strconv"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
)

type proxyMap struct {
	plot   cmap.ConcurrentMap[string, *Proxy]
	parent *ProxyEngine
}

func (sm proxyMap) add(sock string) (*Proxy, bool) {
	sm.plot.SetIfAbsent(sock, &Proxy{
		Endpoint:       sock,
		protocol:       newImmutableProto(),
		lastValidated:  time.UnixMilli(0),
		timesValidated: 0,
		timesBad:       0,
		parent:         sm.parent,
		lock:           stateUnlocked,
	})

	return sm.plot.Get(sock)
}

func (sm proxyMap) delete(sock string) error {
	if _, ok := sm.plot.Get(sock); !ok {
		return errors.New("proxy not found")
	}
	sm.plot.Remove(sock)
	return nil
}

func (sm proxyMap) clear() {
	sm.plot.Clear()
}

func (p5 *ProxyEngine) recycling() int {
	if !p5.GetRecyclingStatus() {
		return 0
	}

	if len(p5.proxyMap.plot) < 1 {
		return 0
	}

	var count int

	p5.proxyMap.mu.RLock()
	defer p5.proxyMap.mu.RUnlock()

	for tuple := range p5.proxyMap.plot.IterBuffered() {
		select {
		case <-p5.ctx.Done():
			return 0
		case p5.Pending <- tuple.Val:
			count++
		default:
			if !printedFull {
				p5.DebugLogger.Print("recycling channel is full!")
				printedFull = true
			}
			continue
		}
	}

	return count
}

var ScaleTimer = time.NewTicker(250 * time.Millisecond)

func (p5 *ProxyEngine) scale() {
	select {
	case <-ScaleTimer.C:
		if p5.pool.IsClosed() {
			return
		}
		if p5.scaler.ScaleAnts(p5.pool, p5.GetTotalValidated(), p5.GetStatistics().Dispensed) {
			msg := strs.Get()
			msg.MustWriteString("job spawner auto scaling, new count: ")
			msg.MustWriteString(strconv.Itoa(p5.pool.Cap()))
			p5.dbgPrint(msg)
		}
	default:
		return
	}
}

func (p5 *ProxyEngine) jobSpawner() {
	if p5.pool.IsClosed() {
		p5.pool.Reboot()
	}

	p5.dbgPrint(simpleString("job spawner started"))
	defer p5.dbgPrint(simpleString("job spawner paused"))

	q := make(chan bool)

	go func() {
		for {
			select {
			case <-p5.ctx.Done():
				q <- true
				return
			case sock := <-p5.Pending:
				p5.scale()
				if err := p5.pool.Submit(sock.validate); err != nil {
					p5.dbgPrint(simpleString(err.Error()))
				}

			default:
				time.Sleep(500 * time.Millisecond)
				count := p5.recycling()
				buf := strs.Get()
				buf.MustWriteString("recycled ")
				buf.MustWriteString(strconv.Itoa(count))
				buf.MustWriteString(" proxies from our map")
				p5.dbgPrint(buf)
			}
		}
	}()

	<-q
	p5.pool.Release()
}
