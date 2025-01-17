// Copyright 2024 Kelvin Clement Mwinuka
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hash

import (
	"context"
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/pkg/constants"
	"github.com/echovault/echovault/pkg/types"
	"math/rand"
	"net"
	"slices"
	"strconv"
	"strings"
)

func handleHSET(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hsetKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	entries := make(map[string]interface{})

	if len(cmd[2:])%2 != 0 {
		return nil, errors.New("each field must have a corresponding value")
	}

	for i := 2; i <= len(cmd)-2; i += 2 {
		entries[cmd[i]] = internal.AdaptType(cmd[i+1])
	}

	if !server.KeyExists(ctx, key) {
		_, err = server.CreateKeyAndLock(ctx, key)
		if err != nil {
			return nil, err
		}
		defer server.KeyUnlock(ctx, key)
		if err = server.SetValue(ctx, key, entries); err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf(":%d\r\n", len(entries))), nil
	}

	if _, err = server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	count := 0
	for field, value := range entries {
		if strings.EqualFold(cmd[0], "hsetnx") {
			if hash[field] == nil {
				hash[field] = value
				count += 1
			}
			continue
		}
		hash[field] = value
		count += 1
	}
	if err = server.SetValue(ctx, key, hash); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", count)), nil
}

func handleHGET(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hgetKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	fields := cmd[2:]

	if !server.KeyExists(ctx, key) {
		return []byte("$-1\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	var value interface{}

	res := fmt.Sprintf("*%d\r\n", len(fields))
	for _, field := range fields {
		value = hash[field]
		if value == nil {
			res += "$-1\r\n"
			continue
		}
		if s, ok := value.(string); ok {
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
			continue
		}
		if d, ok := value.(int); ok {
			res += fmt.Sprintf(":%d\r\n", d)
			continue
		}
		if f, ok := value.(float64); ok {
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
			continue
		}
		res += fmt.Sprintf("$-1\r\n")
	}

	return []byte(res), nil
}

func handleHSTRLEN(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hstrlenKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	fields := cmd[2:]

	if !server.KeyExists(ctx, key) {
		return []byte("$-1\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	var value interface{}

	res := fmt.Sprintf("*%d\r\n", len(fields))
	for _, field := range fields {
		value = hash[field]
		if value == nil {
			res += ":0\r\n"
			continue
		}
		if s, ok := value.(string); ok {
			res += fmt.Sprintf(":%d\r\n", len(s))
			continue
		}
		if f, ok := value.(float64); ok {
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			res += fmt.Sprintf(":%d\r\n", len(fs))
			continue
		}
		if d, ok := value.(int); ok {
			res += fmt.Sprintf(":%d\r\n", len(strconv.Itoa(d)))
			continue
		}
		res += ":0\r\n"
	}

	return []byte(res), nil
}

func handleHVALS(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hvalsKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]

	if !server.KeyExists(ctx, key) {
		return []byte("*0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	res := fmt.Sprintf("*%d\r\n", len(hash))
	for _, val := range hash {
		if s, ok := val.(string); ok {
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
			continue
		}
		if f, ok := val.(float64); ok {
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
			continue
		}
		if d, ok := val.(int); ok {
			res += fmt.Sprintf(":%d\r\n", d)
		}
	}

	return []byte(res), nil
}

func handleHRANDFIELD(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hrandfieldKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]

	count := 1
	if len(cmd) >= 3 {
		c, err := strconv.Atoi(cmd[2])
		if err != nil {
			return nil, errors.New("count must be an integer")
		}
		if c == 0 {
			return []byte("*0\r\n"), nil
		}
		count = c
	}

	withvalues := false
	if len(cmd) == 4 {
		if strings.EqualFold(cmd[3], "withvalues") {
			withvalues = true
		} else {
			return nil, errors.New("result modifier must be withvalues")
		}
	}

	if !server.KeyExists(ctx, key) {
		return []byte("*0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	// If count is the >= hash length, then return the entire hash
	if count >= len(hash) {
		res := fmt.Sprintf("*%d\r\n", len(hash))
		if withvalues {
			res = fmt.Sprintf("*%d\r\n", len(hash)*2)
		}
		for field, value := range hash {
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
			if withvalues {
				if s, ok := value.(string); ok {
					res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
					continue
				}
				if f, ok := value.(float64); ok {
					fs := strconv.FormatFloat(f, 'f', -1, 64)
					res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
					continue
				}
				if d, ok := value.(int); ok {
					res += fmt.Sprintf(":%d\r\n", d)
					continue
				}
			}
		}
		return []byte(res), nil
	}

	// Get all the fields
	var fields []string
	for field, _ := range hash {
		fields = append(fields, field)
	}

	// Pluck fields and return them
	var pluckedFields []string
	var n int
	for i := 0; i < internal.AbsInt(count); i++ {
		n = rand.Intn(len(fields))
		pluckedFields = append(pluckedFields, fields[n])
		// If count is positive, remove the current field from list of fields
		if count > 0 {
			fields = slices.DeleteFunc(fields, func(s string) bool {
				return s == fields[n]
			})
		}
	}

	res := fmt.Sprintf("*%d\r\n", len(pluckedFields))
	if withvalues {
		res = fmt.Sprintf("*%d\r\n", len(pluckedFields)*2)
	}
	for _, field := range pluckedFields {
		res += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
		if withvalues {
			if s, ok := hash[field].(string); ok {
				res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
				continue
			}
			if f, ok := hash[field].(float64); ok {
				fs := strconv.FormatFloat(f, 'f', -1, 64)
				res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
				continue
			}
			if d, ok := hash[field].(int); ok {
				res += fmt.Sprintf(":%d\r\n", d)
				continue
			}
		}
	}

	return []byte(res), nil
}

func handleHLEN(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hlenKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	return []byte(fmt.Sprintf(":%d\r\n", len(hash))), nil
}

func handleHKEYS(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hkeysKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]

	if !server.KeyExists(ctx, key) {
		return []byte("*0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	res := fmt.Sprintf("*%d\r\n", len(hash))
	for field, _ := range hash {
		res += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
	}

	return []byte(res), nil
}

func handleHINCRBY(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hincrbyKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	field := cmd[2]

	var intIncrement int
	var floatIncrement float64

	if strings.EqualFold(cmd[0], "hincrbyfloat") {
		f, err := strconv.ParseFloat(cmd[3], 64)
		if err != nil {
			return nil, errors.New("increment must be a float")
		}
		floatIncrement = f
	} else {
		i, err := strconv.Atoi(cmd[3])
		if err != nil {
			return nil, errors.New("increment must be an integer")
		}
		intIncrement = i
	}

	if !server.KeyExists(ctx, key) {
		if _, err := server.CreateKeyAndLock(ctx, key); err != nil {
			return nil, err
		}
		defer server.KeyUnlock(ctx, key)
		hash := make(map[string]interface{})
		if strings.EqualFold(cmd[0], "hincrbyfloat") {
			hash[field] = floatIncrement
			if err = server.SetValue(ctx, key, hash); err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf("+%s\r\n", strconv.FormatFloat(floatIncrement, 'f', -1, 64))), nil
		} else {
			hash[field] = intIncrement
			if err = server.SetValue(ctx, key, hash); err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf(":%d\r\n", intIncrement)), nil
		}
	}

	if _, err := server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	if hash[field] == nil {
		hash[field] = 0
	}

	switch hash[field].(type) {
	default:
		return nil, fmt.Errorf("value at field %s is not a number", field)
	case int:
		i, _ := hash[field].(int)
		if strings.EqualFold(cmd[0], "hincrbyfloat") {
			hash[field] = float64(i) + floatIncrement
		} else {
			hash[field] = i + intIncrement
		}
	case float64:
		f, _ := hash[field].(float64)
		if strings.EqualFold(cmd[0], "hincrbyfloat") {
			hash[field] = f + floatIncrement
		} else {
			hash[field] = f + float64(intIncrement)
		}
	}

	if err = server.SetValue(ctx, key, hash); err != nil {
		return nil, err
	}

	if f, ok := hash[field].(float64); ok {
		return []byte(fmt.Sprintf("+%s\r\n", strconv.FormatFloat(f, 'f', -1, 64))), nil
	}

	i, _ := hash[field].(int)
	return []byte(fmt.Sprintf(":%d\r\n", i)), nil
}

func handleHGETALL(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hgetallKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]

	if !server.KeyExists(ctx, key) {
		return []byte("*0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	res := fmt.Sprintf("*%d\r\n", len(hash)*2)
	for field, value := range hash {
		res += fmt.Sprintf("$%d\r\n%s\r\n", len(field), field)
		if s, ok := value.(string); ok {
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
		}
		if f, ok := value.(float64); ok {
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			res += fmt.Sprintf("$%d\r\n%s\r\n", len(fs), fs)
		}
		if d, ok := value.(int); ok {
			res += fmt.Sprintf(":%d\r\n", d)
		}
	}

	return []byte(res), nil
}

func handleHEXISTS(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hexistsKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	field := cmd[2]

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	if hash[field] != nil {
		return []byte(":1\r\n"), nil
	}

	return []byte(":0\r\n"), nil
}

func handleHDEL(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := hdelKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	fields := cmd[2:]

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	hash, ok := server.GetValue(ctx, key).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("value at %s is not a hash", key)
	}

	count := 0

	for _, field := range fields {
		if hash[field] != nil {
			delete(hash, field)
			count += 1
		}
	}

	if err = server.SetValue(ctx, key, hash); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", count)), nil
}

func Commands() []types.Command {
	return []types.Command{
		{
			Command:           "hset",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description:       `(HSET key field value [field value ...]) Set update each field of the hash with the corresponding value`,
			Sync:              true,
			KeyExtractionFunc: hsetKeyFunc,
			HandlerFunc:       handleHSET,
		},
		{
			Command:           "hsetnx",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description:       `(HSETNX key field value [field value ...]) Set hash field value only if the field does not exist`,
			Sync:              true,
			KeyExtractionFunc: hsetnxKeyFunc,
			HandlerFunc:       handleHSET,
		},
		{
			Command:           "hget",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description:       `(HGET key field [field ...]) Retrieve the value of each of the listed fields from the hash`,
			Sync:              false,
			KeyExtractionFunc: hgetKeyFunc,
			HandlerFunc:       handleHGET,
		},
		{
			Command:    "hstrlen",
			Module:     constants.HashModule,
			Categories: []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(HSTRLEN key field [field ...]) 
Return the string length of the values stored at the specified fields. 0 if the value does not exist`,
			Sync:              false,
			KeyExtractionFunc: hstrlenKeyFunc,
			HandlerFunc:       handleHSTRLEN,
		},
		{
			Command:           "hvals",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       `(HVALS key) Returns all the values of the hash at key.`,
			Sync:              false,
			KeyExtractionFunc: hvalsKeyFunc,
			HandlerFunc:       handleHVALS,
		},
		{
			Command:           "hrandfield",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       `(HRANDFIELD key [count [WITHVALUES]]) Returns one or more random fields from the hash`,
			Sync:              false,
			KeyExtractionFunc: hrandfieldKeyFunc,
			HandlerFunc:       handleHRANDFIELD,
		},
		{
			Command:           "hlen",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description:       `(HLEN key) Returns the number of fields in the hash`,
			Sync:              false,
			KeyExtractionFunc: hlenKeyFunc,
			HandlerFunc:       handleHLEN,
		},
		{
			Command:           "hkeys",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       `(HKEYS key) Returns all the fields in a hash`,
			Sync:              false,
			KeyExtractionFunc: hkeysKeyFunc,
			HandlerFunc:       handleHKEYS,
		},
		{
			Command:           "hincrbyfloat",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description:       `(HINCRBYFLOAT key field increment) Increment the hash value by the float increment`,
			Sync:              true,
			KeyExtractionFunc: hincrbyKeyFunc,
			HandlerFunc:       handleHINCRBY,
		},
		{
			Command:           "hincrby",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.WriteCategory, constants.FastCategory},
			Description:       `(HINCRBY key field increment) Increment the hash value by the integer increment`,
			Sync:              true,
			KeyExtractionFunc: hincrbyKeyFunc,
			HandlerFunc:       handleHINCRBY,
		},
		{
			Command:           "hgetall",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.SlowCategory},
			Description:       `(HGETALL key) Get all fields and values of a hash`,
			Sync:              false,
			KeyExtractionFunc: hgetallKeyFunc,
			HandlerFunc:       handleHGETALL,
		},
		{
			Command:           "hexists",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description:       `(HEXISTS key field) Returns if field is an existing field in the hash`,
			Sync:              false,
			KeyExtractionFunc: hexistsKeyFunc,
			HandlerFunc:       handleHEXISTS,
		},
		{
			Command:           "hdel",
			Module:            constants.HashModule,
			Categories:        []string{constants.HashCategory, constants.ReadCategory, constants.FastCategory},
			Description:       `(HDEL key field [field ...]) Deletes the specified fields from the hash`,
			Sync:              true,
			KeyExtractionFunc: hdelKeyFunc,
			HandlerFunc:       handleHDEL,
		},
	}
}
