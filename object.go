package glib

/*
#include <stdlib.h>
#include <glib-object.h>

static inline
GType _object_type(GObject* o) {
	return G_OBJECT_TYPE(o);
}

typedef struct {
	GClosure cl;
	gulong h_id;
} GoClosure;

typedef struct {
	GoClosure *cl;
	GValue *ret_val;
	guint n_param;
	const GValue *params;
	gpointer ih;
	gpointer mr_data;
} MarshalParams;

extern void _object_marshal(gpointer mp);  

static inline
void _object_closure_marshal(GClosure* cl, GValue* ret_val, guint n_param,
		const GValue* params, gpointer ih, gpointer mr_data) {
	MarshalParams mp = {(GoClosure*) cl, ret_val, n_param, params, ih, mr_data};
	_object_marshal(&mp);	
}

static inline
GoClosure* _object_closure_new(gpointer p0) {
	GoClosure *cl = (GoClosure*) g_closure_new_simple(sizeof (GoClosure), p0);
	g_closure_set_marshal((GClosure*) cl, _object_closure_marshal);
	return cl;
}

static inline
gulong _signal_connect(GObject* inst, guint sig, GoClosure* cl) {
	return g_signal_connect_closure_by_id(
		inst,
		sig,
		0,
		(GClosure*) cl,
		TRUE
	);
}

static inline
void _signal_emit(const GValue *inst_and_params, guint sig, GValue *ret) {
	return g_signal_emitv(
		inst_and_params,
		sig,
		0,
		ret
	);
}
*/
import "C"

import (
	"reflect"
	"unsafe"
	"fmt"
)

type ObjectGetter interface {
	Object() *Object
}

type Object struct {
	p C.gpointer
}

func (o *Object) g() *C.GObject {
	return (*C.GObject)(o.p)
}

func (o *Object) Pointer() Pointer {
	return Pointer(o.p)
}

func (o *Object) Set(p Pointer) {
	o.p = C.gpointer(p)
}

func (o *Object) Type() Type {
	return Type(C._object_type(o.g()))
}

func (o *Object) Object() *Object {
	return o
}

func (o *Object) Value() *Value {
	v := DefaultValue(o.Type())
	C.g_value_set_object(v.g(), o.p)
	return v
}

// Returns C pointer
func (o *Object) Ref() *Object {
	r := new(Object)
	r.Set(Pointer(C.g_object_ref(o.p)))
	return r
}

func (o *Object) Unref() {
	C.g_object_unref(C.gpointer(o.p))
}

// Returns C pointer
func (o *Object) RefSink() *Object {
	r := new(Object)
	r.Set(Pointer(C.g_object_ref_sink(o.p)))
	return r
}

/*type WeakNotify func(data C.gpointer, o *Object)

// Returns C pointer
func (o *Object) WeakRef(notify WeakNotify, data interface{}) Object {
	v := reflect.ValueOf(data)
	var p uintptr
	if v.Kind() == reflect.Ptr {
		p = v.Pointer()
	} else {
		pv = reflect.New(reflect.TypeOf(data))
		pv.Elem().Set(v)
		p = pv.Pointer()
	}
	...
}*/

func (o *Object) SetProperty(name string, val interface{}) {
	s := C.CString(name)
	defer C.free(unsafe.Pointer(s))
	C.g_object_set_property(o.g(), (*C.gchar)(s),
		ValueOf(val).g())
}

func (o *Object) EmitById(sig SignalId, args ...interface{}) interface{} {
	var sq C.GSignalQuery
	C.g_signal_query(C.guint(sig), &sq)
	if len(args) != int(sq.n_params) {
		panic(fmt.Sprintf(
			"*Object.EmitById " +
			"Number of input parameters #%d doesn't match signal spec #%d",
			len(args), int(sq.n_params),
		))
	}
	prms := make([]Value, len(args)+1)
	prms[0] = *ValueOf(o)
	for i, a := range args {
		prms[i+1] = *ValueOf(a)
	}
	ret := new(Value)
	C._signal_emit(prms[0].g(), C.guint(sig), ret.g())
	fmt.Println("*** emit ***")
	return ret.Get()
}

func (o *Object) Emit(sig_name string, args ...interface{}) interface{} {
	return o.EmitById(SignalLookup(sig_name, o.Type()), args...)
}

type SigHandlerId C.gulong

type sigHandler struct {
	cb, p0 reflect.Value
}

var obj_handlers = make(map[uintptr]map[SigHandlerId]*sigHandler)

func (o *Object) ConnectById(sig SignalId, cb_func, param0 interface{}) {
	cb := reflect.ValueOf(cb_func)
	if cb.Kind() != reflect.Func {
		panic("cb_func isn't a function")
	}
	// Check that function parameters and return value match to signal
	var sq C.GSignalQuery
	C.g_signal_query(C.guint(sig), &sq)
	ft := cb.Type()
	if ft.NumOut() > 1 || ft.NumOut() == 1 && Type(sq.return_type) == TYPE_NONE {
		panic("Number of function return values doesn't match signal spec.")
	}
	poffset := 2
	if param0 == nil {
		// Callback function without param0
		poffset = 1
	}
	n_params := int(sq.n_params)
	if ft.NumIn() != n_params+poffset {
		panic(fmt.Sprintf(
			"Number of callback function parameters #%d isn't equal to #%d",
			ft.NumIn(), n_params+poffset,
		))
	}
	if ft.NumOut() != 0 && !Type(sq.return_type).Match(ft.Out(0)) {
		panic("Type of function return value doesn't match signal spec.")
	}
	fmt.Println(sq.param_types)
	if n_params > 0 {
		pt := (*[1 << 16]Type)(unsafe.Pointer(sq.param_types))[:int(sq.n_params)]
		fmt.Println("(((onnect")
		for i := 0; i < n_params; i++ {
			if !pt[i].Match(ft.In(i + poffset)) {
				panic(fmt.Sprintf(
					"Callback #%d param. type %s doesn't match signal spec %s",
					i+1, ft.In(i), pt[i],
				))
			}
		}
	}
	// Setup closure and connect it to signal
	var gocl *C.GoClosure
	p0 := reflect.ValueOf(param0)
	// Check type of #0 parameter which is set by Connect method
	switch p0.Kind() {
	case reflect.Invalid:
		gocl = C._object_closure_new(nil)
	case reflect.Ptr:
		if !p0.Type().AssignableTo(ft.In(0)) {
			panic(fmt.Sprintf(
				"Callback #0 parameter type: %s doesn't match signal spec: %s",
				ft.In(0), p0,
			))
		}
		gocl = C._object_closure_new(C.gpointer(p0.Pointer()))
	default:
		panic("Callback parameter #0 isn't a pointer nor nil")
	}
	gocl.h_id = C._signal_connect(o.g(), C.guint(sig), gocl)
	oh := obj_handlers[uintptr(o.p)]
	if oh == nil {
		oh = make(map[SigHandlerId]*sigHandler)
		obj_handlers[uintptr(o.p)] = oh
	}
	oh[SigHandlerId(gocl.h_id)] = &sigHandler{cb, p0} // p0 for prevent GC
}

func (o *Object) Connect(sig_name string, cb_func, param0 interface{}) {
	o.ConnectById(SignalLookup(sig_name, o.Type()), cb_func, param0)
}

var (
	ptr_t        = reflect.TypeOf(Pointer(nil))
	ptr_setter_i = reflect.TypeOf((*PointerSetter)(nil)).Elem()
)

func valueFromPointer(p Pointer, t reflect.Type) reflect.Value {
	v := reflect.New(t).Elem()
	*(*Pointer)(unsafe.Pointer(v.UnsafeAddr())) = p
	return v
}

func convertVal(t reflect.Type, v reflect.Value) reflect.Value {
	if v.Type() == ptr_t {
		// v type is Pointer
		var ret reflect.Value
		if t.Implements(ptr_setter_i) {
			// Desired type implements PointerSetter so we are creating
			// new value with desired type and set it from v
			if t.Kind() == reflect.Ptr {
				ret = reflect.New(t.Elem())
			} else {
				ret = reflect.Zero(t)
			}
			ret.Interface().(PointerSetter).Set(v.Interface().(Pointer))
		} else if t != ptr_t && t.Kind() == reflect.Ptr {
			// Input param type is not Pointer but it is some other pointer
			// so we bypass type checking and converting v to desired type.
			ret = valueFromPointer(v.Interface().(Pointer), t)
		}
		return ret
	}
	return v
}

//export _object_marshal
func objectMarshal(mp unsafe.Pointer) {
	fmt.Println("*** marshal ***")
	cmp := (*C.MarshalParams)(mp)
	gc := (*C.GoClosure)(cmp.cl)
	n_param := int(cmp.n_param)
	prms := (*[1 << 16]Value)(unsafe.Pointer(cmp.params))[:n_param]
	h := obj_handlers[uintptr(prms[0].GetPointer())][SigHandlerId(gc.h_id)]
	fmt.Println("*** Doszedl ***")

	if h.p0.Kind() != reflect.Invalid {
		n_param++
	}
	rps := make([]reflect.Value, n_param)
	i := 0
	if h.p0.Kind() != reflect.Invalid {
		// Connect called with param0 != nil
		v := valueFromPointer(Pointer(gc.cl.data), h.p0.Type())
		rps[i] = v
		i++
	}
	cbt := h.cb.Type()
	for _, p := range prms {
		v := reflect.ValueOf(p.Get())
		rps[i] = convertVal(cbt.In(i), v)
		i++
	}
	fmt.Println("rps:", rps)
	fmt.Println("************* Call BEGIN ***************")
	ret := h.cb.Call(rps)
	fmt.Println("*************  Call END  ***************")
	fmt.Println("Return", unsafe.Pointer(cmp.ret_val))
	if cbt.NumOut() == 1 {
		ret_val := (*Value)(cmp.ret_val)
		fmt.Println("ret:", ret[0])
		ret_val.Set(ret[0].Interface())
	}
}

type Params map[string]interface{}

// Returns C pointer
func NewObject(t Type, params Params) *Object {
	if params == nil || len(params) == 0 {
		return &Object{C.g_object_newv(t.g(), 0, nil)}
	}
	p := make([]C.GParameter, len(params))
	i := 0
	for k, v := range params {
		s := C.CString(k)
		defer C.free(unsafe.Pointer(s))
		p[i].name = (*C.gchar)(s)
		p[i].value = *ValueOf(v).g()
		i++
	}
	return &Object{C.g_object_newv(t.g(), C.guint(i), &p[0])}
}
