package pxndscvm

import (
	"fmt"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
	rl "github.com/yunginnanet/Rate5"
)

// Swamp represents a proxy pool
type Swamp struct {
	// ValidSocks5 is a constant stream of verified ValidSocks5 proxies
	ValidSocks5 chan *Proxy
	// ValidSocks4 is a constant stream of verified ValidSocks4 proxies
	ValidSocks4 chan *Proxy
	// ValidSocks4a is a constant stream of verified ValidSocks5 proxies
	ValidSocks4a chan *Proxy

	// Stats holds the Statistics for our swamp
	Stats *Statistics

	Status SwampStatus

	// Pending is a constant stream of proxy strings to be verified
	Pending chan *Proxy

	// see: https://pkg.go.dev/github.com/yunginnanet/Rate5
	useProx *rl.Limiter
	badProx *rl.Limiter

	quit chan bool

	swampmap swampMap

	pool           *ants.Pool
	swampopt       *swampOptions
	runningdaemons int
	mu             *sync.RWMutex
}

var (
	defaultStaleTime = 1 * time.Hour
	defWorkers       = 100
	// Note: I've chosen to use https here exclusively assuring all validated proxies are SSL capable.
	defaultChecks = []string{
		"https://wtfismyip.com/text",
		"https://myexternalip.com/raw",
		"https://ipinfo.io/ip",
		"https://api.ipify.org/",
		"https://icanhazip.com/",
		"https://ifconfig.me/ip",
		"https://www.trackip.net/ip",
		"https://checkip.amazonaws.com/",
	}
)

// https://pkg.go.dev/github.com/yunginnanet/Rate5#Policy
var defUseProx = rl.Policy{
	Window: 60,
	Burst:  2,
}
var defBadProx = rl.Policy{
	Window: 60,
	Burst:  3,
}

// Returns a pointer to our default options (modified and accessed later through concurrent safe getters and setters)
func defOpt() *swampOptions {
	return &swampOptions{
		userAgents:     defaultUserAgents,
		CheckEndpoints: defaultChecks,
		stale:          defaultStaleTime,
		maxWorkers:     defWorkers,

		useProxConfig: defUseProx,
		badProxConfig: defBadProx,

		removeafter: 5,
		recycle:     true,

		validationTimeout: 5,
		debug:             false,
	}
}

// swampOptions holds our configuration for Swamp instances.
// This is implemented as a pointer, and should be interacted with via the setter and getter functions.
type swampOptions struct {
	// stale is the amount of time since verification that qualifies a proxy going stale.
	// if a stale proxy is drawn during the use of our getter functions, it will be skipped.
	stale time.Duration
	// userAgents contains a list of userAgents to be randomly drawn from for proxied requests, this should be supplied via SetUserAgents
	userAgents []string
	// debug when enabled will print results as they come in
	debug bool
	// CheckEndpoints includes web services that respond with (just) the WAN IP of the connection for validation purposes
	CheckEndpoints []string
	// maxWorkers determines the maximum amount of workers used for checking proxies
	maxWorkers int
	// validationTimeout defines the timeout (in seconds) for proxy validation operations.
	// This will apply for both the initial quick check (dial), and the second check (HTTP GET).
	validationTimeout int

	// recycle determines whether or not we recycle proxies pack into the pending channel after we dispense them
	recycle bool
	// remove proxy from recycling after being marked bad this many times
	removeafter int

	// TODO: make getters and setters for these
	useProxConfig rl.Policy
	badProxConfig rl.Policy
}

const (
	stateUnlocked uint32 = iota
	stateLocked
)

// Proxy represents an individual proxy
type Proxy struct {
	// Endpoint is the address:port of the proxy that we connect to
	Endpoint string
	// ProxiedIP is the address that we end up having when making proxied requests through this proxy
	ProxiedIP string
	// Proto is the version/Protocol (currently SOCKS* only) of the proxy
	Proto string
	// LastVerified is the time this proxy was last verified working
	LastVerified time.Time

	// TimesValidated is the amount of times the proxy has been validated.
	TimesValidated int
	// TimesBad is the amount of times the proxy has been marked as bad.
	TimesBad int

	parent *Swamp
	lock   uint32
}

// UniqueKey is an implementation of the Identity interface from Rate5.
// See: https://pkg.go.dev/github.com/yunginnanet/Rate5#Identity
func (sock Proxy) UniqueKey() string {
	return sock.Endpoint
}

// NewDefaultSwamp returns a Swamp with basic options.
// After calling this you can use the various "setters" to change the options before calling Swamp.Start().
func NewDefaultSwamp() *Swamp {
	s := &Swamp{
		ValidSocks5:  make(chan *Proxy, 1000000),
		ValidSocks4:  make(chan *Proxy, 1000000),
		ValidSocks4a: make(chan *Proxy, 1000000),
		Pending:      make(chan *Proxy, 100000000),

		Stats: &Statistics{
			Valid4:    0,
			Valid4a:   0,
			Valid5:    0,
			Dispensed: 0,
			birthday:  time.Now(),
			mu:        &sync.Mutex{},
		},

		swampopt: defOpt(),

		quit:   make(chan bool),
		mu:     &sync.RWMutex{},
		Status: Paused,
	}

	s.swampmap = swampMap{
		plot:   make(map[string]*Proxy),
		mu:     &sync.Mutex{},
		parent: s,
	}

	s.useProx = rl.NewCustomLimiter(s.swampopt.useProxConfig)
	s.badProx = rl.NewCustomLimiter(s.swampopt.badProxConfig)

	var err error
	s.pool, err = ants.NewPool(s.swampopt.maxWorkers, ants.WithOptions(ants.Options{
		ExpiryDuration: 5 * time.Minute,
		PreAlloc: true,
		PanicHandler: s.pondPanic,
	}))

	if err != nil {
		s.dbgPrint(red+"CRITICAL: "+err.Error()+rst)
		panic(err)
	}

	return s
}

func (s *Swamp) pondPanic(p interface{}) {
	fmt.Println("WORKER PANIC! ", p)
	s.dbgPrint(red + "PANIC! " + fmt.Sprintf("%v", p))
}

// defaultUserAgents is a small list of user agents to use during validation.
var defaultUserAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.12; rv:60.0) Gecko/20100101 Firefox/60.0",
	"Mozilla/5.0 (Windows NT 6.2; WOW64; rv:34.0) Gecko/20100101 Firefox/34.0",
	"Mozilla/5.0 (Windows NT 6.2; Win64; x64; rv:24.0) Gecko/20140419 Firefox/24.0 PaleMoon/24.5.0",
	"Mozilla/5.0 (X11; Ubuntu; Linux i686; rv:44.0) Gecko/20100101 Firefox/44.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.9; rv:49.0) Gecko/20100101 Firefox/49.0",
	"Mozilla/5.0 (X11; Ubuntu; Linux i686; rv:55.0) Gecko/20100101 Firefox/55.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.11; rv:47.0) Gecko/20100101 Firefox/--.0",
	"Mozilla/5.0 (Windows NT 6.0; rv:19.0) Gecko/20100101 Firefox/19.0",
	"Mozilla/5.0 (X11; Ubuntu; Linux i686; rv:45.0) Gecko/20100101 Firefox/45.0",
	"Mozilla/5.0 (Windows NT 6.0; WOW64; rv:45.0) Gecko/20100101 Firefox/45.0",
	"Mozilla/5.0 (FreeBSD; Viera; rv:34.0) Gecko/20100101 Firefox/34.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.7; rv:20.0) Gecko/20100101 Firefox/20.0",
	"Mozilla/5.0 (Android 6.0; Mobile; rv:60.0) Gecko/20100101 Firefox/60.0",
	"Mozilla/5.0 (Windows NT 5.1; rv:37.0) Gecko/20100101 Firefox/37.0",
	"Mozilla/5.0 (Windows NT 6.1; WOW64; rv:35.0) Gecko/20100101 Firefox/35.0 evaliant",
	"Mozilla/5.0 (Windows NT 6.1; WOW64; rv:28.0) Gecko/20100101 Firefox/28.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:58.0) Gecko/20100101 Firefox/58.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:60.0) Gecko/20100101 Firefox/60.0",
	"Mozilla/5.0 (Windows NT 10.0; WOW64; rv:45.0) Gecko/20100101 Firefox/45.0",
	"Mozilla/5.0 (Windows NT 6.2; WOW64; rv:41.0) Gecko/20100101 Firefox/41.0",
}
