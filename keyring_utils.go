package main

import (
	"github.com/zalando/go-keyring"
)

const keyringServiceName = "server_manager_cli"

func SetPassword(serverName string, password string) error {
	return keyring.Set(keyringServiceName, serverName, password)
}

func GetPassword(serverName string) (string, error) {
	return keyring.Get(keyringServiceName, serverName)
}

func DeletePassword(serverName string) error {
	return keyring.Delete(keyringServiceName, serverName)
}
