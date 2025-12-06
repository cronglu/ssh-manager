package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const configFile = ".server_manager.json"

func getConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configFile), nil
}

func LoadServers() (map[string]Server, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return make(map[string]Server), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var servers map[string]Server
	err = json.Unmarshal(data, &servers)
	if err != nil {
		// If JSON is invalid, return empty map
		return make(map[string]Server), nil
	}
	return servers, nil
}

func SaveServers(servers map[string]Server) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(servers, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
