package engine

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// The .class fixture is hand-authored rather than compiled: the reader must be
// right against the spec, not against one compiler's output, and a fixture that
// says "this is a constant pool entry" in code is reviewable in a way a hex
// blob is not.

type classBuilder struct {
	b    bytes.Buffer
	n    uint16
	utf8 map[string]uint16
}

func newClassBuilder() *classBuilder {
	return &classBuilder{utf8: map[string]uint16{}}
}

func (cb *classBuilder) str(s string) uint16 {
	if i, ok := cb.utf8[s]; ok {
		return i
	}
	cb.n++
	cb.utf8[s] = cb.n
	binary.Write(&cb.b, binary.BigEndian, uint8(1))
	binary.Write(&cb.b, binary.BigEndian, uint16(len(s)))
	cb.b.WriteString(s)
	return cb.n
}

func (cb *classBuilder) classOf(name string) uint16 {
	ni := cb.str(name) // the Utf8 entry comes first in the pool, and its index first in the call
	cb.n++
	binary.Write(&cb.b, binary.BigEndian, uint8(7))
	binary.Write(&cb.b, binary.BigEndian, ni)
	return cb.n
}

// memberOne emits a field or method entry.
func memberOne(buf *bytes.Buffer, access uint16, name, desc uint16) {
	binary.Write(buf, binary.BigEndian, access)
	binary.Write(buf, binary.BigEndian, name)
	binary.Write(buf, binary.BigEndian, desc)
	binary.Write(buf, binary.BigEndian, uint16(0)) // attribute_count
}

func buildTestClass(t *testing.T) []byte {
	t.Helper()
	cb := newClassBuilder()
	this := cb.classOf("com/acme/Foo")
	super := cb.classOf("java/lang/Object")
	barName, barDesc := cb.str("bar"), cb.str("(ILjava/lang/String;)Ljava/lang/String;")
	initName, initDesc := cb.str("<init>"), cb.str("()V")
	mainName, mainDesc := cb.str("main"), cb.str("([Ljava/lang/String;)V")
	limitName, limitDesc := cb.str("LIMIT"), cb.str("I")
	secretName, secretDesc := cb.str("secret"), cb.str("()V")

	var out bytes.Buffer
	binary.Write(&out, binary.BigEndian, uint32(classMagic))
	binary.Write(&out, binary.BigEndian, uint16(0))  // minor
	binary.Write(&out, binary.BigEndian, uint16(52)) // major
	binary.Write(&out, binary.BigEndian, cb.n+1)     // constant_pool_count = max + 1
	out.Write(cb.b.Bytes())
	binary.Write(&out, binary.BigEndian, uint16(0x0021)) // access: public super
	binary.Write(&out, binary.BigEndian, this)
	binary.Write(&out, binary.BigEndian, super)
	binary.Write(&out, binary.BigEndian, uint16(0)) // interfaces
	binary.Write(&out, binary.BigEndian, uint16(1)) // fields
	memberOne(&out, accPublic|accStatic|accFinal, limitName, limitDesc)
	binary.Write(&out, binary.BigEndian, uint16(4)) // methods
	memberOne(&out, accPublic, barName, barDesc)
	memberOne(&out, accPublic, initName, initDesc)
	memberOne(&out, accPublic|accStatic|accVarargs, mainName, mainDesc)
	memberOne(&out, 0, secretName, secretDesc)      // private -> 4 emitted, 1 dropped
	binary.Write(&out, binary.BigEndian, uint16(0)) // class attributes
	return out.Bytes()
}

func TestClassFileReader(t *testing.T) {
	data := buildTestClass(t)
	cf, err := parseClassFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if cf.Name != "com.acme.Foo" || cf.Super != "java.lang.Object" {
		t.Errorf("name=%q super=%q", cf.Name, cf.Super)
	}
	if got := cf.kindWord(); got != "class" {
		t.Errorf("kindWord = %q", got)
	}
	short := "Foo"
	var sigs []string
	for _, m := range cf.publicMethods() {
		sigs = append(sigs, prettyMethod(short, m))
	}
	want := map[string]bool{
		"String bar(int, String)":              true,
		"Foo()":                                true,
		"static void main(String[]) (varargs)": true,
	}
	got := map[string]bool{}
	for _, s := range sigs {
		got[s] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("missing %q; got %v", w, sigs)
		}
	}
	for _, m := range cf.publicMethods() {
		if m.Name == "secret" {
			t.Error("private method leaked")
		}
	}
	fields := cf.publicFields()
	if len(fields) != 1 || prettyField(fields[0]) != "static final int LIMIT" {
		t.Errorf("fields = %v", fields)
	}
}

func TestClassFileRejectsGarbage(t *testing.T) {
	data := buildTestClass(t)
	if _, err := parseClassFile([]byte("not a class at all")); err == nil {
		t.Error("non-class accepted")
	}
	if _, err := parseClassFile(data[:len(data)/2]); err == nil {
		t.Error("truncated class accepted")
	}
	// A constant pool count inflated beyond the remaining bytes must fail, not allocate.
	bad := append([]byte{}, data...)
	bad[8] = 0xEA
	bad[9] = 0x60 // cp_count = 0xEA60, far more than the file could carry
	if _, err := parseClassFile(bad); err == nil {
		t.Error("implausible constant pool accepted")
	}
}

func TestDescriptorRendering(t *testing.T) {
	if got := prettyType("I"); got != "int" {
		t.Errorf("prettyType(I) = %q", got)
	}
	if got := prettyType("[[Ljava/lang/String;"); got != "String[][]" {
		t.Errorf("prettyType(2d String) = %q", got)
	}
	if got := prettyType("Lcom/acme/Weird$Inner;"); got != "Weird$Inner" {
		t.Errorf("prettyType(inner) = %q", got)
	}
	if got := prettyType("Lfoo"); got != "Lfoo" {
		t.Errorf("malformed descriptor must pass through, got %q", got)
	}
	if got := prettyArgs("IJZ"); got != "int, long, boolean" {
		t.Errorf("prettyArgs = %q", got)
	}
	if got := prettyArgs("(bogus"); got != "" {
		t.Errorf("prettyArgs(malformed) = %q, want empty stop", got)
	}
	m := classMember{Access: accPublic, Name: "of", Desc: "(Lacme/Book;[I)Ljava/util/List;"}
	if got := prettyMethod("Shelf", m); got != "List of(Book, int[])" {
		t.Errorf("prettyMethod = %q", got)
	}
	if got := (classFile{Access: accInterface}).kindWord(); got != "interface" {
		t.Errorf("interface kindWord = %q", got)
	}
	if !strings.Contains(prettyMethod("Foo", classMember{Access: accPublic, Name: "toString", Desc: "()Ljava/lang/String;"}), "String toString()") {
		t.Error("toString render")
	}
}
