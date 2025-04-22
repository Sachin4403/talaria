package service

import (
	"fmt"
	"github.com/go-kit/kit/sd"
)

// Registrars is a aggregate sd.Registrar that allows allows composite registration and deregistration.
// Keys in this map type will be service advertisements or instances, e.g. "host.com:8080" or "https://foobar.com"
type Registrars map[string]sd.Registrar

func (r Registrars) Register() {
	for k, v := range r {
		fmt.Printf("Register with %s ", k)
		v.Register()
		fmt.Printf("Registered with %s ", k)
	}
}

func (r Registrars) Deregister() {
	for k, v := range r {
		fmt.Printf("Deregister with %s ", k)
		v.Deregister()
		fmt.Printf("Deregistered with %s ", k)
	}
}

func (r Registrars) Has(key string) bool {
	for k, v := range r {
		fmt.Printf("registrars %s", k)
		fmt.Println(v)
	}
	fmt.Println("end registrars ")

	_, ok := r[key]
	return ok
}

func (r Registrars) Len() int {
	fmt.Println("length of registrars", len(r))
	return len(r)
}

func (r *Registrars) Add(key string, v sd.Registrar) {
	if *r == nil {
		*r = make(Registrars)
	}

	(*r)[key] = v
}
