package engine

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// A minimal, bounded Java class-file reader (FIX_PLAN §5.2 tier 2): constant
// pool, access flags, field and method tables, nothing else. The whole point is
// reliability without weight — stdlib only, no bytecode execution, no third
// dependency, and the only structure it must understand well enough to *trust*
// is the one that keeps member names and descriptors honest. A malformed class
// is an error string, never a panic and never a partial member list.
//
// Signatures carry types but not parameter names: names live in MethodParameters
// and LocalVariableTable attributes, which are optional and frequently stripped.
// The plan accepted that; the sources tier fills the gap when a jar has them.

const classMagic = 0xCAFEBABE

// Access flags from JVMS §4.6/4.7 — the subset this reader displays.
const (
	accPublic     = 0x0001
	accFinal      = 0x0010
	accStatic     = 0x0008
	accInterface  = 0x0200
	accAbstract   = 0x0400
	accSynthetic  = 0x1000
	accAnnotation = 0x2000
	accBridge     = 0x0040
	accVarargs    = 0x0080
)

type classMember struct {
	Access uint16
	Name   string
	Desc   string // field or method descriptor, verbatim from the class file
}

type classFile struct {
	Name    string // binary name, dots (com.acme.Foo)
	Access  uint16
	Super   string // empty for Object and interfaces
	Members []classMember
}

// publicMethods lists the methods an outside caller can reach: public, not
// synthetic, not a bridge. Constructors come back named "<init>"; callers
// render them with the class name.
func (cf classFile) publicMethods() []classMember {
	var out []classMember
	for _, m := range cf.Members {
		if m.Access&accPublic != 0 && m.Access&accSynthetic == 0 && m.Access&accBridge == 0 &&
			strings.Contains(m.Desc, "(") {
			out = append(out, m)
		}
	}
	return out
}

func (cf classFile) publicFields() []classMember {
	var out []classMember
	for _, m := range cf.Members {
		if m.Access&accPublic != 0 && m.Access&accSynthetic == 0 && !strings.Contains(m.Desc, "(") {
			out = append(out, m)
		}
	}
	return out
}

func (cf classFile) kindWord() string {
	switch {
	case cf.Access&accAnnotation != 0:
		return "@interface"
	case cf.Access&accInterface != 0:
		return "interface"
	case cf.Access&accEnum != 0:
		return "enum"
	case strings.HasSuffix(cf.Name, "Exception") || strings.HasSuffix(cf.Name, "Error") ||
		strings.HasSuffix(cf.Name, "Throwable"):
		return "exception"
	default:
		return "class"
	}
}

const accEnum = 0x4000

type cfReader struct {
	b   []byte
	pos int
	err error
}

func (r *cfReader) u1() uint32 {
	if r.err != nil {
		return 0
	}
	if r.pos+1 > len(r.b) {
		r.err = errors.New("class file truncated")
		return 0
	}
	v := uint32(r.b[r.pos])
	r.pos++
	return v
}

func (r *cfReader) u2() uint32 {
	if r.err != nil {
		return 0
	}
	if r.pos+2 > len(r.b) {
		r.err = errors.New("class file truncated")
		return 0
	}
	v := uint32(binary.BigEndian.Uint16(r.b[r.pos:]))
	r.pos += 2
	return v
}

func (r *cfReader) u4() uint32 {
	if r.err != nil {
		return 0
	}
	if r.pos+4 > len(r.b) {
		r.err = errors.New("class file truncated")
		return 0
	}
	v := binary.BigEndian.Uint32(r.b[r.pos:])
	r.pos += 4
	return v
}

func (r *cfReader) skip(n int) {
	if r.err != nil {
		return
	}
	if n < 0 || r.pos+n > len(r.b) {
		r.err = errors.New("class file truncated")
		return
	}
	r.pos += n
}

// parseClassFile reads one .class payload. It bounds every table count against
// the bytes that remain, so a hostile or corrupt file fails instead of claiming
// a million-entry constant pool.
func parseClassFile(data []byte) (classFile, error) {
	var cf classFile
	r := &cfReader{b: data}
	if len(data) < 10 || binary.BigEndian.Uint32(data) != classMagic {
		return cf, errors.New("not a class file")
	}
	r.skip(4)
	r.u2() // minor
	r.u2() // major
	cpCount := r.u2()
	if r.err != nil {
		return cf, r.err
	}
	if cpCount == 0 || int(cpCount)*2 > len(data) { // every entry costs >= 2 bytes
		return cf, fmt.Errorf("implausible constant pool size %d", cpCount)
	}

	utf8 := make([]string, cpCount)
	classRef := make([]uint32, cpCount) // tag 7 -> name index
	for i := uint32(1); i < cpCount && r.err == nil; i++ {
		tag := r.u1()
		switch tag {
		case 1: // Utf8
			n := r.u2()
			if r.pos+int(n) > len(r.b) {
				r.err = errors.New("class file truncated")
				break
			}
			utf8[i] = string(r.b[r.pos : r.pos+int(n)])
			r.pos += int(n)
		case 7: // Class
			classRef[i] = r.u2()
		case 9, 10, 11, 12, 17, 18: // field/method/iface refs, name-and-type, dynamic: u2+u2
			r.skip(4)
		case 8, 16, 19, 20: // string, method type, module, package: one index
			r.skip(2)
		case 15: // method handle: u1 + u2
			r.skip(3)
		case 3, 4: // Integer, Float
			r.skip(4)
		case 5, 6: // Long, Double take two slots
			r.skip(8)
			i++
		default:
			r.err = fmt.Errorf("unknown constant pool tag %d at %d", tag, i)
		}
	}
	if r.err != nil {
		return cf, r.err
	}

	cf.Access = uint16(r.u2())
	thisIdx := r.u2()
	superIdx := r.u2()
	nameOf := func(ref uint32) string {
		if ref == 0 || ref >= cpCount {
			return ""
		}
		nameIdx := classRef[ref]
		if nameIdx == 0 || nameIdx >= cpCount {
			return ""
		}
		return strings.ReplaceAll(utf8[nameIdx], "/", ".")
	}
	cf.Name = nameOf(thisIdx)
	cf.Super = nameOf(superIdx)

	nIfaces := r.u2()
	if int(nIfaces)*2 > len(data)-r.pos {
		return cf, errors.New("implausible interfaces count")
	}
	r.skip(int(nIfaces) * 2)

	member := func() {
		count := r.u2()
		if r.err != nil {
			return
		}
		// Each member is at minimum flags(2)+name(2)+desc(2)+attrs(2) = 8 bytes.
		if int(count)*8 > len(data)-r.pos {
			r.err = errors.New("implausible member count")
			return
		}
		for i := uint32(0); i < count && r.err == nil; i++ {
			var m classMember
			m.Access = uint16(r.u2())
			ni, di := r.u2(), r.u2()
			nAttr := r.u2()
			if r.err != nil {
				return
			}
			if ni < cpCount {
				m.Name = utf8[ni]
			}
			if di < cpCount {
				m.Desc = utf8[di]
			}
			for a := uint32(0); a < nAttr && r.err == nil; a++ {
				r.u2() // attribute name index
				l := r.u4()
				if int(l) > len(data)-r.pos {
					r.err = errors.New("implausible attribute length")
					return
				}
				r.skip(int(l))
			}
			cf.Members = append(cf.Members, m)
		}
	}
	member() // fields
	member() // methods
	if r.err != nil {
		return classFile{}, r.err
	}
	return cf, nil
}

// --- descriptor rendering ---------------------------------------------------------

// prettyMethod renders a method descriptor as a Java-ish signature. Parameter
// names are not in descriptors, so `bar(ILjava/lang/String;)V` becomes
// `void bar(int, String)` — types honest, names absent, and the output says
// where it came from so nobody mistakes it for source.
func prettyMethod(classShort string, m classMember) string {
	open := strings.IndexByte(m.Desc, '(')
	if open < 0 {
		return m.Name
	}
	args := m.Desc[open+1 : strings.IndexByte(m.Desc, ')')]
	ret := prettyType(m.Desc[strings.IndexByte(m.Desc, ')')+1:])
	name := m.Name
	if name == "<init>" {
		name = classShort
		ret = ""
	} else if name == "<clinit>" {
		return ""
	}
	var parts []string
	if m.Access&accStatic != 0 {
		parts = append(parts, "static")
	}
	if ret != "" {
		parts = append(parts, ret)
	}
	parts = append(parts, name+"("+prettyArgs(args)+")")
	if m.Access&accVarargs != 0 && len(parts) > 0 {
		parts[len(parts)-1] += " (varargs)"
	}
	return strings.Join(parts, " ")
}

func prettyField(m classMember) string {
	pre := ""
	if m.Access&accStatic != 0 && m.Access&accFinal != 0 {
		pre = "static final "
	} else if m.Access&accStatic != 0 {
		pre = "static "
	}
	return pre + prettyType(m.Desc) + " " + m.Name
}

func prettyArgs(desc string) string {
	var args []string
	for i := 0; i < len(desc); {
		t, n := readType(desc[i:])
		if n == 0 {
			return strings.Join(args, ", ") // malformed: stop, keep what parsed
		}
		args = append(args, t)
		i += n
	}
	return strings.Join(args, ", ")
}

// prettyType renders a full descriptor ("([Ljava/lang/String;)") or returns the
// input unchanged when it is not a descriptor, so partial renders stay readable.
func prettyType(desc string) string {
	t, n := readType(desc)
	if n != len(desc) {
		return desc
	}
	return t
}

// readType consumes one type from the front of desc, returning its rendering and
// the bytes consumed; n == 0 means malformed input.
func readType(desc string) (string, int) {
	arr := 0
	i := 0
	for i < len(desc) && desc[i] == '[' {
		arr++
		i++
	}
	if i >= len(desc) {
		return "", 0
	}
	var base string
	switch desc[i] {
	case 'B':
		base = "byte"
	case 'C':
		base = "char"
	case 'D':
		base = "double"
	case 'F':
		base = "float"
	case 'I':
		base = "int"
	case 'J':
		base = "long"
	case 'S':
		base = "short"
	case 'Z':
		base = "boolean"
	case 'V':
		base = "void"
	case 'L':
		end := strings.IndexByte(desc[i:], ';')
		if end < 0 {
			return "", 0
		}
		name := desc[i+1 : i+end]
		if j := strings.LastIndexByte(name, '/'); j >= 0 {
			name = name[j+1:] // short names; the jar's package is already in the row
		}
		base = name
		i += end + 1
		return base + strings.Repeat("[]", arr), i
	default:
		return "", 0
	}
	return base + strings.Repeat("[]", arr), i + 1 // scalars consume one byte
}
