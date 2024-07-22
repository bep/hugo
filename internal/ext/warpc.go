package ext

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gohugoio/hugo/common/hugio"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed wasm/quickjs.wasm
var quickjsWasm []byte

type IDGetter interface {
	GetID() uint32
}

type Dispatcher[Q, R IDGetter] interface {
	Execute(ctx context.Context, q Q) (R, error)
	Close() error
}

func (p *dispatcherPool[Q, R]) getDispatcher() *dispatcher[Q, R] {
	i := int(p.counter.Add(1)) % len(p.dispatchers)
	return p.dispatchers[i]
}

func (p *dispatcherPool[Q, R]) Close() error {
	return p.close()
}

type dispatcher[Q, R IDGetter] struct {
	zero    R
	counter atomic.Uint32

	mu    sync.RWMutex
	encMu sync.Mutex

	pending map[uint32]*call[Q, R]

	inOut *inOut

	shutdown bool
	closing  bool

	close func() error
}

type inOut struct {
	sync.Mutex
	stdin  hugio.ReadWriteCloser
	stdout hugio.ReadWriteCloser
	dec    *json.Decoder
	enc    *json.Encoder
}

var ErrShutdown = fmt.Errorf("dispatcher is shutting down")

var timerPool = sync.Pool{}

func getTimer(d time.Duration) *time.Timer {
	if v := timerPool.Get(); v != nil {
		timer := v.(*time.Timer)
		timer.Reset(d)
		return timer
	}
	return time.NewTimer(d)
}

func putTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	timerPool.Put(t)
}

// Execute sends a request to the dispatcher and waits for the response.
func (p *dispatcherPool[Q, R]) Execute(ctx context.Context, q Q) (R, error) {
	d := p.getDispatcher()
	if q.GetID() == 0 {
		return d.zero, errors.New("ID must not be 0 (note that this must be unique within the current request set time window)")
	}

	call, err := d.newCall(q)
	if err != nil {
		return d.zero, err
	}

	if err := d.send(call); err != nil {
		return d.zero, err
	}

	timer := getTimer(30 * time.Second)
	defer putTimer(timer)

	select {
	case call = <-call.Done:
	case <-timer.C:
		return d.zero, errors.New("timeout")
	}

	if call.Error != nil {
		return d.zero, call.Error
	}

	return call.Response, nil
}

func (d *dispatcher[Q, R]) newCall(q Q) (*call[Q, R], error) {
	call := &call[Q, R]{
		Done:    make(chan *call[Q, R], 1),
		Request: q,
	}

	if d.shutdown || d.closing {
		call.Error = ErrShutdown
		call.done()
		return call, nil
	}

	d.mu.Lock()
	d.pending[q.GetID()] = call
	d.mu.Unlock()

	return call, nil
}

func (d *dispatcher[Q, R]) send(call *call[Q, R]) error {
	d.mu.RLock()
	if d.closing || d.shutdown {
		d.mu.RUnlock()
		return ErrShutdown
	}
	d.mu.RUnlock()

	d.encMu.Lock()
	defer d.encMu.Unlock()
	err := d.inOut.enc.Encode(call.Request)
	if err != nil {
		return err
	}
	return nil
}

func (d *dispatcher[Q, R]) input() {
	var inputErr error

	for d.inOut.dec.More() {
		var r R
		if err := d.inOut.dec.Decode(&r); err != nil {
			inputErr = err
			break
		}

		d.mu.Lock()
		call, found := d.pending[r.GetID()]
		if !found {
			d.mu.Unlock()
			panic(fmt.Errorf("call with ID %d not found", r.GetID()))
		}
		delete(d.pending, r.GetID())
		d.mu.Unlock()
		call.Response = r
		call.done()
	}

	// Terminate pending calls.
	d.shutdown = true
	if inputErr != nil {
		isEOF := inputErr == io.EOF || strings.Contains(inputErr.Error(), "already closed")
		if isEOF {
			if d.closing {
				inputErr = ErrShutdown
			} else {
				inputErr = io.ErrUnexpectedEOF
			}
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	for _, call := range d.pending {
		call.Error = inputErr
		call.done()
	}
}

type call[Q, R any] struct {
	Request  Q
	Response R
	Error    error
	Done     chan *call[Q, R]
}

func (call *call[Q, R]) done() {
	select {
	case call.Done <- call:
	default:
	}
}

type Options struct {
	Ctx context.Context

	CompileModule    func(ctx context.Context, r wazero.Runtime, io []*inOut) (func() error, error)
	CompilationCache wazero.CompilationCache
	PoolSize         int
}

func Start[Q, R IDGetter](opts Options) (Dispatcher[Q, R], error) {
	if opts.PoolSize == 0 {
		opts.PoolSize = 1
	}

	return newDispatcher[Q, R](opts)
}

type dispatcherPool[Q, R IDGetter] struct {
	counter     atomic.Uint32
	dispatchers []*dispatcher[Q, R]
	close       func() error
}

func newDispatcher[Q, R IDGetter](opts Options) (*dispatcherPool[Q, R], error) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.CompileModule == nil {
		return nil, errors.New("InstansiateModule is required")
	}
	ctx := opts.Ctx

	runtimeConfig := wazero.NewRuntimeConfig()

	if opts.CompilationCache != nil {
		runtimeConfig = runtimeConfig.WithCompilationCache(opts.CompilationCache)
	}

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntimeWithConfig(opts.Ctx, runtimeConfig)

	// Instantiate WASI, which implements system I/O such as console output.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return nil, err
	}

	inOuts := make([]*inOut, opts.PoolSize)
	for i := 0; i < opts.PoolSize; i++ {
		var stdin, stdout hugio.ReadWriteCloser

		stdin = hugio.NewPipeReadWriteCloser()
		stdout = hugio.NewPipeReadWriteCloser()

		inOuts[i] = &inOut{
			stdin:  stdin,
			stdout: stdout,
			dec:    json.NewDecoder(stdout),
			enc:    json.NewEncoder(stdin),
		}
	}

	run, err := opts.CompileModule(ctx, r, inOuts)
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		// This will block until stdin is closed.
		err := run()
		if err != nil {
			panic(err)
		}
		close(done)
	}()

	dispatchers := make([]*dispatcher[Q, R], len(inOuts))
	for i := 0; i < len(inOuts); i++ {
		d := &dispatcher[Q, R]{
			pending: make(map[uint32]*call[Q, R]),
			inOut:   inOuts[i],
		}
		go d.input()
		dispatchers[i] = d
	}

	close := func() error {
		for _, d := range dispatchers {
			d.closing = true
			if err := d.inOut.stdin.Close(); err != nil {
				return err
			}
			if err := d.inOut.stdout.Close(); err != nil {
				return err
			}
		}

		// We need to wait for the WebAssembly instances to finish executing before we can close the runtime.
		<-done

		return r.Close(ctx)
	}

	dp := &dispatcherPool[Q, R]{
		dispatchers: dispatchers,
		close:       close,
	}

	return dp, nil
}

func printStackTrace(w io.Writer) {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	fmt.Fprintf(w, "%s", buf)
}

func compileFunc(name string, wasm []byte, needsQuickJSProvider bool) func(ctx context.Context, r wazero.Runtime, inouts []*inOut) (func() error, error) {
	return func(ctx context.Context, r wazero.Runtime, inouts []*inOut) (func() error, error) {
		compiledModule, err := r.CompileModule(ctx, wasm)
		if err != nil {
			return nil, err
		}

		var compiledQuickJS wazero.CompiledModule
		if needsQuickJSProvider {
			compiledQuickJS, err = r.CompileModule(ctx, quickjsWasm)
			if err != nil {
				return nil, err
			}
		}

		return func() error {
			var wg sync.WaitGroup
			for i, c := range inouts {
				name := fmt.Sprintf("%s_%d", name, i)
				c := c

				wg.Add(1)
				go func() {
					defer wg.Done()
					configBase := wazero.NewModuleConfig().WithStderr(os.Stderr).WithStartFunctions()
					config := configBase.WithName(name)
					if needsQuickJSProvider {
						// TODO1 else, configure with stdout etc.
						qmod, err := r.InstantiateModule(ctx, compiledQuickJS, configBase.WithName("").WithStdout(c.stdout).WithStdin(c.stdin))
						if err != nil {
							panic(err)
						}
						ctx = experimental.WithImportResolver(ctx,
							func(name string) api.Module {
								if name == "javy_quickjs_provider_v2" {
									return qmod
								}
								return nil
							})

					}

					mod, err := r.InstantiateModule(ctx, compiledModule, config)
					if err != nil {
						panic(err)
					}
					if _, err := mod.ExportedFunction("_start").Call(ctx); err != nil {
						panic(err)
					}
				}()
			}
			wg.Wait()
			return nil
		}, nil
	}
}

// TODO1 notes
/*

QuickJS native JSON intrinsic https://github.com/bytecodealliance/javy/blob/main/crates/javy/src/config.rs
Whether to override the implementation of JSON.parse and JSON.stringify
*/
