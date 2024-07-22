package ext

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tetratelabs/wazero"
)

//go:embed wasm/greet.wasm
var greetWasm []byte

type person struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
}

func (p person) GetID() uint32 {
	return p.ID
}

func TestKatex(t *testing.T) {
	opts := Options{
		CompileModule: compileFunc("renderkatex", katexWasm, true),
	}

	d, err := Start[KatexInput, KatexOutput](opts)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	ctx := context.Background()

	input := KatexInput{
		ID:          uint32(32),
		Expression:  "c = \\pm\\sqrt{a^2 + b^2}",
		DisplayMode: true,
	}

	result, err := d.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.ID != input.ID {
		t.Fatalf("%d vs %d", result.ID, input.ID)
	}
}

func TestGreet(t *testing.T) {
	opts := Options{
		CompileModule: compileFunc("greet", greetWasm, true),
		PoolSize:      2,
	}

	d, err := Start[person, greeting](opts)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	ctx := context.Background()

	inputPerson := person{
		Name: "Person",
	}

	for i := 0; i < 20; i++ {
		inputPerson.ID = uint32(i + 1)
		g, err := d.Execute(ctx, inputPerson)
		if err != nil {
			t.Fatal(err)
		}
		if g.Greeting != "Hello Person!" {
			t.Fatalf("got: %v", g)
		}
		if g.ID != inputPerson.ID {
			t.Fatalf("%d vs %d", g.ID, inputPerson.ID)
		}

	}
}

func TestGreetParallel(t *testing.T) {
	opts := Options{
		CompileModule: compileFunc("greet", greetWasm, true),
		PoolSize:      4,
	}
	d, err := Start[*person, greeting](opts)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	var wg sync.WaitGroup

	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			ctx := context.Background()

			for j := 0; j < 5; j++ {
				base := i * 100
				id := uint32(base + j)

				inputPerson := &person{
					Name: fmt.Sprintf("Person %d", id),
					ID:   id,
				}
				g, err := d.Execute(ctx, inputPerson)
				if err != nil {
					t.Error(err)
					return
				}
				if g.Greeting != fmt.Sprintf("Hello Person %d!", id) {
					t.Errorf("got: %v", g)
					return
				}
				if g.ID != inputPerson.ID {
					t.Errorf("%d vs %d", g.ID, inputPerson.ID)
					return
				}
			}
		}(i)

	}

	wg.Wait()
}

func compilationCache(t testing.TB) wazero.CompilationCache {
	tempDir := t.TempDir()

	compilationCache, err := wazero.NewCompilationCacheWithDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	return compilationCache
}

func TestKatexParallel(t *testing.T) {
	opts := Options{
		CompileModule:    compileFunc("katex", katexWasm, true),
		PoolSize:         6,
		CompilationCache: compilationCache(t),
	}
	d, err := Start[KatexInput, KatexOutput](opts)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	var wg sync.WaitGroup

	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			ctx := context.Background()

			for j := 0; j < 1; j++ {
				base := i * 100
				id := uint32(base + j)

				input := katexInputTemplate
				input.ID = id

				result, err := d.Execute(ctx, input)
				if err != nil {
					t.Error(err)
					return
				}

				if result.ID != input.ID {
					t.Errorf("%d vs %d", result.ID, input.ID)
					return
				}
			}
		}(i)

	}

	wg.Wait()
}

func BenchmarkExecuteKatex(b *testing.B) {
	opts := Options{
		CompileModule: compileFunc("katex", katexWasm, true),
	}
	d, err := Start[*KatexInput, KatexOutput](opts)
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()

	ctx := context.Background()

	input := &katexInputTemplate

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input.ID = uint32(i + 1)
		result, err := d.Execute(ctx, input)
		if err != nil {
			b.Fatal(err)
		}

		if result.ID != input.ID {
			b.Fatalf("%d vs %d", result.ID, input.ID)
		}

	}
}

func BenchmarkKatexStartStop(b *testing.B) {
	tempDir := b.TempDir()

	compilationCache, err := wazero.NewCompilationCacheWithDir(tempDir)
	if err != nil {
		b.Fatal(err)
	}

	optsTemplate := Options{
		CompileModule: compileFunc("katex", katexWasm, true),
		// CompileCacheDir: tempDir,
		CompilationCache: compilationCache,
	}

	runBench := func(b *testing.B, opts Options) {
		for i := 0; i < b.N; i++ {
			d, err := Start[KatexInput, KatexOutput](opts)
			if err != nil {
				b.Fatal(err)
			}
			if err := d.Close(); err != nil {
				b.Fatal(err)
			}
		}
	}

	for _, poolSize := range []int{1, 8, 16} {

		name := fmt.Sprintf("PoolSize%d", poolSize)

		b.Run(name, func(b *testing.B) {
			opts := optsTemplate
			opts.PoolSize = poolSize
			runBench(b, opts)
		})

	}
}

var katexInputTemplate = KatexInput{
	Expression:  "c = \\pm\\sqrt{a^2 + b^2}",
	DisplayMode: true,
	Output:      "html",
}

func BenchmarkExecuteKatexPara(b *testing.B) {
	optsTemplate := Options{
		CompileModule:    compileFunc("katex", katexWasm, true),
		CompilationCache: compilationCache(b),
	}

	runBench := func(b *testing.B, opts Options) {
		d, err := Start[KatexInput, KatexOutput](opts)
		if err != nil {
			b.Fatal(err)
		}
		defer d.Close()

		ctx := context.Background()

		b.ResetTimer()

		var id atomic.Uint32
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				input := katexInputTemplate
				input.ID = id.Add(1)
				result, err := d.Execute(ctx, input)
				if err != nil {
					b.Fatal(err)
				}
				if result.ID != input.ID {
					b.Fatalf("%d vs %d", result.ID, input.ID)
				}
			}
		})
	}

	for _, poolSize := range []int{1, 8, 16} {
		name := fmt.Sprintf("PoolSize%d", poolSize)

		b.Run(name, func(b *testing.B) {
			opts := optsTemplate
			opts.PoolSize = poolSize
			runBench(b, opts)
		})
	}
}

func BenchmarkExecuteGreet(b *testing.B) {
	opts := Options{
		CompileModule: compileFunc("greet", greetWasm, true),
	}
	d, err := Start[*person, greeting](opts)
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()

	ctx := context.Background()

	input := &person{
		Name: "Person",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input.ID = uint32(i + 1)
		result, err := d.Execute(ctx, input)
		if err != nil {
			b.Fatal(err)
		}

		if result.ID != input.ID {
			b.Fatalf("%d vs %d", result.ID, input.ID)
		}

	}
}

func BenchmarkExecuteGreetPara(b *testing.B) {
	opts := Options{
		CompileModule: compileFunc("greet", greetWasm, true),
		PoolSize:      8,
	}

	d, err := Start[person, greeting](opts)
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()

	ctx := context.Background()

	inputTemplate := person{
		Name: "Person",
	}

	b.ResetTimer()

	var id atomic.Uint32
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			input := inputTemplate
			input.ID = id.Add(1)
			result, err := d.Execute(ctx, input)
			if err != nil {
				b.Fatal(err)
			}
			if result.ID != input.ID {
				b.Fatalf("%d vs %d", result.ID, input.ID)
			}
		}
	})
}

type greeting struct {
	ID       uint32 `json:"id"`
	Greeting string `json:"greeting"`
}

func (g greeting) GetID() uint32 {
	return g.ID
}
