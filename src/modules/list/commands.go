package list

import (
	"context"
	"errors"
	"fmt"
	"github.com/kelvinmwinuka/memstore/src/utils"
	"math"
	"net"
	"reflect"
	"strings"
)

const (
	OK = "+OK\r\n\n"
)

type Plugin struct {
	name        string
	commands    []utils.Command
	categories  []string
	description string
}

var ListModule Plugin

func (p Plugin) Name() string {
	return p.name
}

func (p Plugin) Commands() []utils.Command {
	return p.commands
}

func (p Plugin) Description() string {
	return p.description
}

func (p Plugin) HandleCommand(ctx context.Context, cmd []string, server utils.Server, conn *net.Conn) ([]byte, error) {
	c := strings.ToLower(cmd[0])

	switch {
	default:
		return nil, errors.New("command unknown")
	case c == "llen":
		return handleLLen(ctx, cmd, server)

	case c == "lindex":
		return handleLIndex(ctx, cmd, server)

	case c == "lrange":
		return handleLRange(ctx, cmd, server)

	case c == "lset":
		return handleLSet(ctx, cmd, server)

	case c == "ltrim":
		return handleLTrim(ctx, cmd, server)

	case c == "lrem":
		return handleLRem(ctx, cmd, server)

	case c == "lmove":
		return handleLMove(ctx, cmd, server)

	case utils.Contains[string]([]string{"lpush", "lpushx"}, c):
		return handleLPush(ctx, cmd, server)

	case utils.Contains[string]([]string{"rpush", "rpushx"}, c):
		return handleRPush(ctx, cmd, server)

	case utils.Contains[string]([]string{"lpop", "rpop"}, c):
		return handlePop(ctx, cmd, server)
	}
}

func handleLLen(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 2 {
		return nil, errors.New("wrong number of args for LLEN command")
	}

	if !server.KeyExists(cmd[1]) {
		// Key, does not exist, return
		return nil, errors.New("LLEN command on non-list item")
	}

	server.KeyRLock(ctx, cmd[1])
	list, ok := server.GetValue(cmd[1]).([]interface{})
	server.KeyRUnlock(cmd[1])

	if !ok {
		return nil, errors.New("LLEN command on non-list item")
	}

	return []byte(fmt.Sprintf(":%d\r\n\n", len(list))), nil
}

func handleLIndex(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 3 {
		return nil, errors.New("wrong number of args for LINDEX command")
	}

	index, ok := utils.AdaptType(cmd[2]).(int64)

	if !ok {
		return nil, errors.New("index must be an integer")
	}

	if !server.KeyExists(cmd[1]) {
		return nil, errors.New("LINDEX command on non-list item")
	}

	server.KeyRLock(ctx, cmd[1])
	list, ok := server.GetValue(cmd[1]).([]interface{})
	server.KeyRUnlock(cmd[1])

	if !ok {
		return nil, errors.New("LINDEX command on non-list item")
	}

	if !(index >= 0 && int(index) < len(list)) {
		return nil, errors.New("index must be within list range")
	}

	return []byte(fmt.Sprintf("+%s\r\n\n", list[index])), nil
}

func handleLRange(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New("wrong number of arguments for LRANGE command")
	}

	start, startOk := utils.AdaptType(cmd[2]).(int64)
	end, endOk := utils.AdaptType(cmd[3]).(int64)

	if !startOk || !endOk {
		return nil, errors.New("both start and end indices must be integers")
	}

	if !server.KeyExists(cmd[1]) {
		return nil, errors.New("LRANGE command on non-list item")
	}

	server.KeyRLock(ctx, cmd[1])
	list, ok := server.GetValue(cmd[1]).([]interface{})
	server.KeyRUnlock(cmd[1])

	if !ok {
		return nil, errors.New("type cannot be returned with LRANGE command")
	}

	// Make sure start is within range
	if !(start >= 0 && int(start) < len(list)) {
		return nil, errors.New("start index not within list range")
	}

	// Make sure end is within range, or is -1 otherwise
	if !((end >= 0 && int(end) < len(list)) || end == -1) {
		return nil, errors.New("end index must be within list range or -1")
	}

	var bytes []byte

	// If end is -1, read list from start to the end of the list
	if end == -1 {
		bytes = []byte("*" + fmt.Sprint(len(list)-int(start)) + "\r\n")
		for i := int(start); i < len(list); i++ {
			str := fmt.Sprintf("%v", list[i])
			bytes = append(bytes, []byte("$"+fmt.Sprint(len(str))+"\r\n"+str+"\r\n")...)
		}
		bytes = append(bytes, []byte("\n")...)
		return bytes, nil
	}

	// Make sure start and end are not equal to each other
	if start == end {
		return nil, errors.New("start and end indices cannot be equal")
	}

	// If end is not -1:
	//	1) If end is larger than start, return slice from start -> end
	//	2) If end is smaller than start, return slice from end -> start
	bytes = []byte("*" + fmt.Sprint(int(math.Abs(float64(start-end)))+1) + "\r\n")

	i := start
	j := end + 1
	if start > end {
		j = end - 1
	}

	for i != j {
		str := fmt.Sprintf("%v", list[i])
		bytes = append(bytes, []byte("$"+fmt.Sprint(len(str))+"\r\n"+str+"\r\n")...)
		if start < end {
			i++
		} else {
			i--
		}

	}

	bytes = append(bytes, []byte("\n")...)

	return bytes, nil
}

func handleLSet(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New("wrong number of arguments for LSET command")
	}

	if !server.KeyExists(cmd[1]) {
		return nil, errors.New("LSET command on non-list item")
	}

	server.KeyLock(ctx, cmd[1])
	list, ok := server.GetValue(cmd[1]).([]interface{})

	if !ok {
		server.KeyUnlock(cmd[1])
		return nil, errors.New("LSET command on non-list item")
	}

	index, ok := utils.AdaptType(cmd[2]).(int64)

	fmt.Printf("LSET INDEX: `%v`, OK: %v\n", reflect.TypeOf(index), ok)

	if !ok {
		server.KeyUnlock(cmd[1])
		return nil, errors.New("index must be an integer")
	}

	if !(index >= 0 && int(index) < len(list)) {
		server.KeyUnlock(cmd[1])
		return nil, errors.New("index must be within range")
	}

	list[index] = utils.AdaptType(cmd[3])
	server.SetValue(ctx, cmd[1], list)
	server.KeyUnlock(cmd[1])

	return []byte(OK), nil
}

func handleLTrim(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New("wrong number of args for command LTRIM")
	}

	start, startOk := utils.AdaptType(cmd[2]).(int64)
	end, endOk := utils.AdaptType(cmd[3]).(int64)

	if !startOk || !endOk {
		return nil, errors.New("start and end indices must be integers")
	}

	if end < start && end != -1 {
		return nil, errors.New("end index must be greater than start index or -1")
	}

	if !server.KeyExists(cmd[1]) {
		return nil, errors.New("LTRIM command on non-list item")
	}

	server.KeyLock(ctx, cmd[1])
	list, ok := server.GetValue(cmd[1]).([]interface{})

	if !ok {
		return nil, errors.New("LTRIM command on non-list item")
	}

	if !(start >= 0 && int(start) < len(list)) {
		return nil, errors.New("start index must be within list boundary")
	}

	if end == -1 || int(end) > len(list) {
		server.SetValue(ctx, cmd[1], list[start:])
		server.KeyUnlock(cmd[1])
		return []byte(OK), nil
	}

	server.SetValue(ctx, cmd[1], list[start:end])
	server.KeyUnlock(cmd[1])
	return []byte(OK), nil
}

func handleLRem(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New("wrong number of arguments for LREM command")
	}

	value := cmd[3]
	count, ok := utils.AdaptType(cmd[2]).(int64)

	if !ok {
		return nil, errors.New("count must be an integer")
	}

	absoluteCount := math.Abs(float64(count))

	if !server.KeyExists(cmd[1]) {
		return nil, errors.New("LREM command on non-list item")
	}

	server.KeyLock(ctx, cmd[1])
	list, ok := server.GetValue(cmd[1]).([]interface{})

	if !ok {
		return nil, errors.New("LREM command on non-list item")
	}

	switch {
	default:
		// Count is zero, keep list the same
	case count > 0:
		// Start from the head
		for i := 0; i < len(list); i++ {
			if absoluteCount == 0 {
				break
			}
			if fmt.Sprintf("%v", list[i]) == value {
				list[i] = nil
				absoluteCount -= 1
			}
		}
	case count < 0:
		// Start from the tail
		for i := len(list) - 1; i >= 0; i-- {
			if absoluteCount == 0 {
				break
			}
			if fmt.Sprintf("%v", list[i]) == value {
				list[i] = nil
				absoluteCount -= 1
			}
		}
	}

	list = utils.Filter[interface{}](list, func(elem interface{}) bool {
		return elem != nil
	})

	server.SetValue(ctx, cmd[1], list)
	server.KeyUnlock(cmd[1])

	return []byte(OK), nil
}

func handleLMove(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 5 {
		return nil, errors.New("wrong number of arguments for LMOVE command")
	}

	whereFrom := strings.ToLower(cmd[3])
	whereTo := strings.ToLower(cmd[4])

	if !utils.Contains[string]([]string{"left", "right"}, whereFrom) || !utils.Contains[string]([]string{"left", "right"}, whereTo) {
		return nil, errors.New("wherefrom and whereto arguments must be either LEFT or RIGHT")
	}

	if !server.KeyExists(cmd[1]) || !server.KeyExists(cmd[2]) {
		return nil, errors.New("both source and destination must be lists")
	}

	// TODO: Make this atomic
	server.KeyLock(ctx, cmd[1])
	server.KeyLock(ctx, cmd[2])
	source, sourceOk := server.GetValue(cmd[1]).([]interface{})
	destination, destinationOk := server.GetValue(cmd[2]).([]interface{})

	if !sourceOk || !destinationOk {
		return nil, errors.New("both source and destination must be lists")
	}

	switch whereFrom {
	case "left":
		server.SetValue(ctx, cmd[1], append([]interface{}{}, source[1:]...))
		if whereTo == "left" {
			server.SetValue(ctx, cmd[2], append(source[0:1], destination...))
		} else if whereTo == "right" {
			server.SetValue(ctx, cmd[2], append(destination, source[0]))
		}
	case "right":
		server.SetValue(ctx, cmd[1], append([]interface{}{}, source[:len(source)-1]...))
		if whereTo == "left" {
			server.SetValue(ctx, cmd[2], append(source[len(source)-1:], destination...))
		} else if whereTo == "right" {
			server.SetValue(ctx, cmd[2], append(destination, source[len(source)-1]))
		}
	}

	server.KeyUnlock(cmd[1])
	server.KeyUnlock(cmd[2])

	return []byte(OK), nil
}

func handleLPush(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 {
		return nil, fmt.Errorf("wrong number of arguments for %s command", strings.ToUpper(cmd[0]))
	}

	newElems := []interface{}{}

	for _, elem := range cmd[2:] {
		newElems = append(newElems, utils.AdaptType(elem))
	}

	key := cmd[1]

	if !server.KeyExists(key) {
		switch strings.ToLower(cmd[0]) {
		case "lpushx":
			return nil, fmt.Errorf("%s command on non-list item", cmd[0])
		default:
			// TODO: Retry CreateKeyAndLock until we obtain the key lock
			server.CreateKeyAndLock(ctx, key)
			server.SetValue(ctx, key, []interface{}{})
		}
	}

	defer server.KeyUnlock(key)

	currentList := server.GetValue(key)

	l, ok := currentList.([]interface{})

	if !ok {
		return nil, fmt.Errorf("%s command on non-list item", cmd[0])
	}

	server.SetValue(ctx, key, append(newElems, l...))
	return []byte(OK), nil
}

func handleRPush(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 {
		return nil, fmt.Errorf("wrong number of arguments for %s command", strings.ToUpper(cmd[0]))
	}

	newElems := []interface{}{}

	for _, elem := range cmd[2:] {
		newElems = append(newElems, utils.AdaptType(elem))
	}

	if !server.KeyExists(cmd[1]) {
		switch strings.ToLower(cmd[0]) {
		case "rpushx":
			return nil, fmt.Errorf("%s command on non-list item", cmd[0])
		default:
			// TODO: Retry CreateKeyAndLock until we managed to obtain the key
			server.CreateKeyAndLock(ctx, cmd[1])
			server.SetValue(ctx, cmd[1], []interface{}{})
		}
	}

	defer server.KeyUnlock(cmd[1])

	currentList := server.GetValue(cmd[1])

	l, ok := currentList.([]interface{})

	if !ok {
		return nil, errors.New("RPUSH command on non-list item")
	}

	server.SetValue(ctx, cmd[1], append(l, newElems...))
	return []byte(OK), nil
}

func handlePop(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 2 {
		return nil, fmt.Errorf("wrong number of args for %s command", strings.ToUpper(cmd[0]))
	}

	if !server.KeyExists(cmd[1]) {
		return nil, fmt.Errorf("%s command on non-list item", strings.ToUpper(cmd[0]))
	}

	if !server.KeyExists(cmd[1]) {
		return nil, fmt.Errorf("%s command on non-list item", strings.ToUpper(cmd[0]))
	}

	server.KeyLock(ctx, cmd[1])
	defer server.KeyUnlock(cmd[1])

	list, ok := server.GetValue(cmd[1]).([]interface{})

	if !ok {
		return nil, fmt.Errorf("%s command on non-list item", strings.ToUpper(cmd[0]))
	}

	switch strings.ToLower(cmd[0]) {
	default:
		server.SetValue(ctx, cmd[1], list[1:])
		return []byte(fmt.Sprintf("+%v\r\n\n", list[0])), nil
	case "rpop":
		server.SetValue(ctx, cmd[1], list[:len(list)-1])
		return []byte(fmt.Sprintf("+%v\r\n\n", list[len(list)-1])), nil
	}

}

func NewModule() Plugin {
	ListModule := Plugin{
		name: "ListCommands",
		commands: []utils.Command{
			{
				Command:     "lpush",
				Categories:  []string{},
				Description: "(LPUSH key value1 [value2]) Prepends one or more values to the beginning of a list, creates the list if it does not exist.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 3 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "lpushx",
				Categories:  []string{},
				Description: "(LPUSHX key value) Prepends a value to the beginning of a list only if the list exists.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 3 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "lpop",
				Categories:  []string{},
				Description: "(LPOP key) Removes and returns the first element of a list.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 2 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "llen",
				Categories:  []string{},
				Description: "(LLEN key) Return the length of a list.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 2 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "lrange",
				Categories:  []string{},
				Description: "(LRANGE key start end) Return a range of elements between the given indices.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 4 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "lindex",
				Categories:  []string{},
				Description: "(LINDEX key index) Gets list element by index.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 3 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "lset",
				Categories:  []string{},
				Description: "(LSET key index value) Sets the value of an element in a list by its index.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 4 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "ltrim",
				Categories:  []string{},
				Description: "(LTRIM key start end) Trims a list to the specified range.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 4 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "lrem",
				Categories:  []string{},
				Description: "(LREM key count value) Remove elements from list.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 4 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "lmove",
				Categories:  []string{},
				Description: "(LMOVE source destination <LEFT | RIGHT> <LEFT | RIGHT>) Move element from one list to the other specifying left/right for both lists.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 5 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1], cmd[2]}, nil
				},
			},
			{
				Command:     "rpop",
				Categories:  []string{},
				Description: "(RPOP key) Removes and gets the last element in a list.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 2 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "rpush",
				Categories:  []string{},
				Description: "(RPUSH key value [value2]) Appends one or multiple elements to the end of a list.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 3 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:     "rpushx",
				Categories:  []string{},
				Description: "(RPUSHX key value) Appends an element to the end of a list, only if the list exists.",
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 3 {
						return nil, errors.New("wrong number of arguments")
					}
					return []string{cmd[1]}, nil
				},
			},
		},
		description: "Handle List commands",
	}
	return ListModule
}
