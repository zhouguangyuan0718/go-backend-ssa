package compiler_test

import (
	"context"
	"errors"
	"go-backend-ssa/compiler"
	"strings"
	"testing"
)

func TestWalkDiagnosticsPreserved(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			depReq := request("internal/abi", `package abi;func FuncPCABI0(f any)uintptr;func FuncPCABIInternal(f any)uintptr`)
			depReq.Options.Standard = true
			if arch == "arm64" {
				depReq.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
			}
			dep, err := compiler.Compile(context.Background(), depReq)
			if err != nil {
				t.Fatal(err)
			}
			for _, stage := range []compiler.Stage{"", compiler.Built, compiler.EarlyOptimized} {
				t.Run(string(stage), func(t *testing.T) {
					defer func() {
						if failure := recover(); failure != nil {
							t.Fatalf("source diagnostics became panic: %v", failure)
						}
					}()
					r := request("p", `package p
import "internal/abi"
func Fn0()
func Fn1(){}
var FnExpr func()
func Invalid(){
 _=abi.FuncPCABIInternal(Fn0)
 _=abi.FuncPCABI0(Fn1)
 _=abi.FuncPCABI0(FnExpr)
}
func Valid()int{return 7}
`)
					r.Target = depReq.Target
					r.Imports = map[string][]byte{"internal/abi": dep.Archive}
					r.SymABIs = []byte("def p.Fn0 ABI0\n")
					r.Options.Workers = 4
					r.Options.Stage = stage
					if stage == "" {
						var result *compiler.Result
						result, err = compiler.Compile(context.Background(), r)
						if result != nil {
							t.Fatal("invalid package produced archive")
						}
					} else {
						var result *compiler.Package
						result, err = compiler.Build(context.Background(), r)
						if result != nil {
							t.Fatal("invalid package produced SSA")
						}
					}
					var diagnostics *compiler.Error
					if !errors.As(err, &diagnostics) || len(diagnostics.Diagnostics) != 3 {
						t.Fatal("lost source diagnostics", err)
					}
					for i, d := range diagnostics.Diagnostics {
						if d.Line != uint(7+i) || !strings.Contains(d.Message, "internal/abi.FuncPCABI") {
							t.Fatal("wrong diagnostic", d)
						}
					}
					r.Files[0].Data = []byte("package p;func Valid()int{return 7}")
					if stage == "" {
						_, err = compiler.Compile(context.Background(), r)
					} else {
						_, err = compiler.Build(context.Background(), r)
					}
					if err != nil {
						t.Fatal("failed compilation poisoned new instance", err)
					}
				})
			}
		})
	}
}
