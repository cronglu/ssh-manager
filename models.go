package main

type Server struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Host    string `json:"host"`
	User    string `json:"user"`
	Bastion string `json:"bastion,omitempty"`
}
