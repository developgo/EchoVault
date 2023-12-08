package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

type Server interface {
	KeyLock(ctx context.Context, key string) (bool, error)
	KeyUnlock(key string)
	KeyRLock(ctx context.Context, key string) (bool, error)
	KeyRUnlock(key string)
	KeyExists(key string) bool
	CreateKeyAndLock(ctx context.Context, key string) (bool, error)
	GetValue(key string) interface{}
	SetValue(ctx context.Context, key string, value interface{})
}

type plugin struct {
	name        string
	commands    []string
	description string
}

var Plugin plugin

func (p *plugin) Name() string {
	return p.name
}

func (p *plugin) Commands() []string {
	return p.commands
}

func (p *plugin) Description() string {
	return p.description
}

func (p *plugin) HandleCommandWithConnection(ctx context.Context, cmd []string, server interface{}, conn *net.Conn) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (p *plugin) HandleCommand(ctx context.Context, cmd []string, server interface{}) ([]byte, error) {
	switch strings.ToLower(cmd[0]) {
	default:
		return nil, errors.New("command unknown")
	case "setrange":
		return handleSetRange(ctx, cmd, server.(Server))
	case "strlen":
		return handleStrLen(ctx, cmd, server.(Server))
	case "substr":
		return handleSubStr(ctx, cmd, server.(Server))
	case "getrange":
		return handleSubStr(ctx, cmd, server.(Server))
	}
}

func handleSetRange(ctx context.Context, cmd []string, server Server) ([]byte, error) {
	if len(cmd[1:]) != 3 {
		return nil, errors.New("wrong number of args for SETRANGE command")
	}

	key := cmd[1]

	offset, ok := AdaptType(cmd[2]).(int64)
	if !ok {
		return nil, errors.New("offset must be integer")
	}

	newStr := cmd[3]

	if !server.KeyExists(key) {
		if _, err := server.CreateKeyAndLock(ctx, key); err != nil {
			return nil, err
		}
		server.SetValue(ctx, key, newStr)
		server.KeyUnlock(key)
		return []byte(fmt.Sprintf(":%d\r\n\n", len(newStr))), nil
	}

	str, ok := server.GetValue(key).(string)
	if !ok {
		return nil, fmt.Errorf("value at key %s is not a string", key)
	}

	if _, err := server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(key)

	if offset >= int64(len(str)) {
		newStr = str + newStr
		server.SetValue(ctx, key, newStr)
		return []byte(fmt.Sprintf(":%d\r\n\n", len(newStr))), nil
	}

	if offset < 0 {
		newStr = newStr + str
		server.SetValue(ctx, key, newStr)
		return []byte(fmt.Sprintf(":%d\r\n\n", len(newStr))), nil
	}

	if offset == 0 {
		newStr = newStr + strings.Join(strings.Split(str, "")[1:], "")
		server.SetValue(ctx, key, newStr)
		return []byte(fmt.Sprintf(":%d\r\n\n", len(newStr))), nil
	}

	if offset == int64(len(str))-1 {
		newStr = strings.Join(strings.Split(str, "")[0:len(str)-1], "") + newStr
		server.SetValue(ctx, key, newStr)
		return []byte(fmt.Sprintf(":%d\r\n\n", len(newStr))), nil
	}

	strArr := strings.Split(str, "")
	newStrArr := append(strArr[0:offset], append(strings.Split(newStr, ""), strArr[offset+1:]...)...)

	newStr = strings.Join(newStrArr, "")
	server.SetValue(ctx, key, newStr)

	return []byte(fmt.Sprintf(":%d\r\n\n", len(newStr))), nil
}

func handleStrLen(ctx context.Context, cmd []string, server Server) ([]byte, error) {
	if len(cmd[1:]) != 1 {
		return nil, errors.New("wrong number of args for STRLEN command")
	}

	key := cmd[1]

	if !server.KeyExists(key) {
		return []byte(":0\r\n\n"), nil
	}

	if _, err := server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(key)

	value, ok := server.GetValue(key).(string)

	if !ok {
		return nil, fmt.Errorf("value at key %s is not a string", key)
	}

	return []byte(fmt.Sprintf(":%d\r\n\n", len(value))), nil
}

func handleSubStr(ctx context.Context, cmd []string, server Server) ([]byte, error) {
	if len(cmd[1:]) != 3 {
		return nil, errors.New("wrong number of args for SUBSTR command")
	}

	key := cmd[1]

	start, startOk := AdaptType(cmd[2]).(int64)
	end, endOk := AdaptType(cmd[3]).(int64)
	reversed := false

	if !startOk || !endOk {
		return nil, errors.New("start and end indices must be integers")
	}

	if !server.KeyExists(key) {
		return nil, fmt.Errorf("key %s does not exist", key)
	}

	if _, err := server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(key)

	value, ok := server.GetValue(key).(string)

	if !ok {
		return nil, fmt.Errorf("value at key %s is not a string", key)
	}

	if end >= 0 {
		end += 1
	}

	if start < 0 {
		start = int64(len(value)) + start
	}
	if end < 0 {
		end = int64(len(value)) + end
	}

	if end > int64(len(value)) {
		end = int64(len(value))
	}

	if start > end {
		reversed = true
		start, end = end, start
	}

	str := value[start:end]

	if reversed {
		res := ""
		for i := len(str) - 1; i >= 0; i-- {
			res = res + string(str[i])
		}
		str = res
	}

	return []byte(fmt.Sprintf("$%d\r\n%s\r\n\n", len(str), str)), nil
}

func init() {
	Plugin.name = "StringCommands"
	Plugin.commands = []string{
		"setrange", // (SETRANGE key offset value) Overwrites part of a string value with another by offset. Creates the key if it doesn't exist.
		"strlen",   // (STRLEN key) Returns length of the key's value if it's a string.
		"substr",   // (SUBSTR key start end) Returns a substring from the string value.
		"getrange", // (GETRANGE key start end) Returns a substring from the string value.
	}
	Plugin.description = "Handle basic STRING commands"
}