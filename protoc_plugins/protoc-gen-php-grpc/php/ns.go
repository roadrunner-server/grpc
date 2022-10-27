// MIT License
//
// Copyright (c) 2022 SpiralScout
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package php

import (
	"bytes"
	"fmt"
	"strings"

	desc "google.golang.org/protobuf/types/descriptorpb"
	plugin "google.golang.org/protobuf/types/pluginpb"
)

// manages internal name representation of the package
type ns struct {
	// Package defines file package.
	Package string

	// Root namespace of the package
	Namespace string

	// Declares the namespaces of messages used as input and output types for service methods
	MessageNamespaces map[string]string
}

// newNamespace creates new work namespace.
func newNamespace(req *plugin.CodeGeneratorRequest, file *desc.FileDescriptorProto, service *desc.ServiceDescriptorProto) *ns {
	ns := &ns{
		Package:           deref(file.Package),
		Namespace:         namespace(file.Package, "\\"),
		MessageNamespaces: make(map[string]string),
	}

	if file.Options != nil && file.Options.PhpNamespace != nil {
		ns.Namespace = *file.Options.PhpNamespace
	}

	for k := range service.Method {
		ns.registerMessageNamespace(req, service.Method[k].InputType)
		ns.registerMessageNamespace(req, service.Method[k].OutputType)
	}

	return ns
}

// Registers the namespace of a message used as an input or output type for a service method.
func (ns *ns) registerMessageNamespace(req *plugin.CodeGeneratorRequest, msg *string) {
	if msg == nil {
		return
	}

	chunks := strings.Split(*msg, ".")
	pkg := strings.Join(chunks[:len(chunks)-1], ".")

	result := bytes.NewBuffer(nil)
	for _, p := range chunks[:len(chunks)-1] {
		result.WriteString(identifier(p, ""))
		result.WriteString(`\`)
	}

	if pkg == "."+ns.Package {
		// root package
		return
	}

	for _, f := range req.ProtoFile {
		if pkg == "."+deref(f.Package) {
			if f.Options != nil && f.Options.PhpNamespace != nil {
				// custom message namespace
				ns.MessageNamespaces[pkg] = *f.Options.PhpNamespace
				return
			}
		}
	}

	ns.MessageNamespaces[pkg] = strings.Trim(result.String(), `\`)
}

// resolve message alias
func (ns *ns) resolve(msg *string) string {
	chunks := strings.Split(*msg, ".")
	pkg := strings.Join(chunks[:len(chunks)-1], ".")

	if pkg == "."+ns.Package {
		// root message
		return identifier(chunks[len(chunks)-1], "")
	}

	for iPkg, ns := range ns.MessageNamespaces {
		if pkg == iPkg {
			// use last namespace chunk
			identifier := identifier(chunks[len(chunks)-1], "")

			return fmt.Sprintf(
				`\%s\%s`,
				ns,
				resolveReserved(identifier, pkg),
			)
		}
	}

	// fully clarified name (fallback)
	return "\\" + namespace(msg, "\\")
}

func deref(val *string) string {
	if val == nil {
		return ""
	}
	return *val
}
