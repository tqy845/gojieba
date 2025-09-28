package gojieba

/*
#cgo CXXFLAGS: -I./deps/cppjieba/include -I./deps/cppjieba/deps/limonp/include -DLOGGING_LEVEL=LL_WARNING -O3 -Wno-deprecated -Wno-unused-variable -std=c++11
#include <stdlib.h>
#include "jieba.h"
*/
import "C"
import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

type TokenizeMode int

const (
	DefaultMode TokenizeMode = iota
	SearchMode
)

type Word struct {
	Str   string
	Start int
	End   int
}

type Jieba struct {
	jieba C.Jieba
	freed int32
}

var (
	sharedInstance     *Jieba
	sharedInstanceOnce sync.Once
	sharedInstanceLock sync.RWMutex
)

func NewJieba(paths ...string) *Jieba {
	dictpaths := getDictPaths(paths...)

	// check if the dictionary files exist
	for _, path := range dictpaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			panic(fmt.Sprintf("Dictionary file does not exist: %s", path))
		}
	}

	dpath, hpath, upath, ipath, spath := C.CString(dictpaths[0]), C.CString(dictpaths[1]), C.CString(dictpaths[2]), C.CString(dictpaths[3]), C.CString(dictpaths[4])
	defer C.free(unsafe.Pointer(dpath))
	defer C.free(unsafe.Pointer(hpath))
	defer C.free(unsafe.Pointer(upath))
	defer C.free(unsafe.Pointer(ipath))
	defer C.free(unsafe.Pointer(spath))
	jieba := &Jieba{
		C.NewJieba(
			dpath,
			hpath,
			upath,
			ipath,
			spath,
		),
		0,
	}
	// set finalizer to free the memory when the object is garbage collected
	runtime.SetFinalizer(jieba, (*Jieba).Free)
	return jieba
}

// GetSharedInstance 返回一个共享的Jieba实例，可以显著减少内存使用
//
// 使用场景：
// - 当你需要频繁创建和销毁Jieba实例时
// - 当你的应用中多个地方需要使用分词功能时
// - 当你希望减少内存占用时
//
// 注意事项：
// - 共享实例是线程安全的，可以在多个goroutine中同时使用
// - 不要对共享实例调用Free()方法
// - 共享实例会在程序结束时自动释放
// - 第一次调用时会创建实例，后续调用返回同一个实例
//
// 示例：
//
//	jieba := GetSharedInstance()
//	words := jieba.Cut("我来到北京清华大学", true)
//	// 不需要调用 jieba.Free()
func GetSharedInstance(paths ...string) *Jieba {
	sharedInstanceOnce.Do(func() {
		sharedInstance = NewJieba(paths...)
		// 移除finalizer，因为共享实例在程序生命周期内一直存在
		runtime.SetFinalizer(sharedInstance, nil)
	})

	sharedInstanceLock.RLock()
	defer sharedInstanceLock.RUnlock()

	if atomic.LoadInt32(&sharedInstance.freed) != 0 {
		panic("Shared Jieba instance has been freed")
	}

	return sharedInstance
}

func (x *Jieba) Free() {
	if atomic.CompareAndSwapInt32(&x.freed, 0, 1) { // only free once
		C.FreeJieba(x.jieba)
		x.jieba = nil
		// 清除finalizer，避免重复释放
		runtime.SetFinalizer(x, nil)
	}
}

func (x *Jieba) checkFreed() {
	if atomic.LoadInt32(&x.freed) != 0 {
		panic("Jieba instance has been freed")
	}
}

func (x *Jieba) Cut(s string, hmm bool) []string {
	x.checkFreed()
	c_int_hmm := 0
	if hmm {
		c_int_hmm = 1
	}
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	var words **C.char = C.Cut(x.jieba, cstr, C.int(c_int_hmm))
	defer C.FreeWords(words)
	res := cstrings(words)
	return res
}

func (x *Jieba) CutAll(s string) []string {
	x.checkFreed()
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	var words **C.char = C.CutAll(x.jieba, cstr)
	defer C.FreeWords(words)
	res := cstrings(words)
	return res
}

func (x *Jieba) CutForSearch(s string, hmm bool) []string {
	x.checkFreed()
	c_int_hmm := 0
	if hmm {
		c_int_hmm = 1
	}
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	var words **C.char = C.CutForSearch(x.jieba, cstr, C.int(c_int_hmm))
	defer C.FreeWords(words)
	res := cstrings(words)
	return res
}

func (x *Jieba) Tag(s string) []string {
	x.checkFreed()
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	var words **C.char = C.Tag(x.jieba, cstr)
	defer C.FreeWords(words)
	res := cstrings(words)
	return res
}

func (x *Jieba) AddWord(s string) {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	C.AddWord(x.jieba, cstr)
}

func (x *Jieba) AddWordEx(s string, freq int, tag string) {
	cstr := C.CString(s)
	ctag := C.CString(tag)
	defer C.free(unsafe.Pointer(ctag))
	defer C.free(unsafe.Pointer(cstr))
	C.AddWordEx(x.jieba, cstr, C.int(freq), ctag)
}

func (x *Jieba) RemoveWord(s string) {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	C.RemoveWord(x.jieba, cstr)
}

func (x *Jieba) Tokenize(s string, mode TokenizeMode, hmm bool) []Word {
	c_int_hmm := 0
	if hmm {
		c_int_hmm = 1
	}
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	var words *C.Word = C.Tokenize(x.jieba, cstr, C.TokenizeMode(mode), C.int(c_int_hmm))
	defer C.free(unsafe.Pointer(words))
	return convertWords(s, words)
}

type WordWeight struct {
	Word   string
	Weight float64
}

func (x *Jieba) Extract(s string, topk int) []string {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	var words **C.char = C.Extract(x.jieba, cstr, C.int(topk))
	res := cstrings(words)
	defer C.FreeWords(words)
	return res
}

func (x *Jieba) ExtractWithWeight(s string, topk int) []WordWeight {
	cstr := C.CString(s)
	defer C.free(unsafe.Pointer(cstr))
	words := C.ExtractWithWeight(x.jieba, cstr, C.int(topk))
	p := unsafe.Pointer(words)
	res := cwordweights((*C.struct_CWordWeight)(p))
	defer C.FreeWordWeights(words)
	return res
}

func cwordweights(x *C.struct_CWordWeight) []WordWeight {
	var s []WordWeight
	eltSize := unsafe.Sizeof(*x)
	for (*x).word != nil {
		ww := WordWeight{
			C.GoString(((C.struct_CWordWeight)(*x)).word),
			float64((*x).weight),
		}
		s = append(s, ww)
		x = (*C.struct_CWordWeight)(unsafe.Pointer(uintptr(unsafe.Pointer(x)) + eltSize))
	}
	return s
}
