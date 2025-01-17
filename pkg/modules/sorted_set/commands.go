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

package sorted_set

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/echovault/echovault/internal"
	"github.com/echovault/echovault/internal/sorted_set"
	"github.com/echovault/echovault/pkg/constants"
	"github.com/echovault/echovault/pkg/types"
	"math"
	"net"
	"slices"
	"strconv"
	"strings"
)

func handleZADD(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zaddKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]

	var updatePolicy interface{} = nil
	var comparison interface{} = nil
	var changed interface{} = nil
	var incr interface{} = nil

	// Find the first valid score and this will be the start of the score/member pairs
	var membersStartIndex int
	for i := 0; i < len(cmd); i++ {
		if membersStartIndex != 0 {
			break
		}
		switch internal.AdaptType(cmd[i]).(type) {
		case string:
			if slices.Contains([]string{"-inf", "+inf"}, strings.ToLower(cmd[i])) {
				membersStartIndex = i
			}
		case float64:
			membersStartIndex = i
		case int:
			membersStartIndex = i
		}
	}

	if membersStartIndex < 2 || len(cmd[membersStartIndex:])%2 != 0 {
		return nil, errors.New("score/member pairs must be float/string")
	}

	var members []sorted_set.MemberParam

	for i := 0; i < len(cmd[membersStartIndex:]); i++ {
		if i%2 != 0 {
			continue
		}
		score := internal.AdaptType(cmd[membersStartIndex:][i])
		switch score.(type) {
		default:
			return nil, errors.New("invalid score in score/member list")
		case string:
			var s float64
			if strings.ToLower(score.(string)) == "-inf" {
				s = math.Inf(-1)
				members = append(members, sorted_set.MemberParam{
					Value: sorted_set.Value(cmd[membersStartIndex:][i+1]),
					Score: sorted_set.Score(s),
				})
			}
			if strings.ToLower(score.(string)) == "+inf" {
				s = math.Inf(1)
				members = append(members, sorted_set.MemberParam{
					Value: sorted_set.Value(cmd[membersStartIndex:][i+1]),
					Score: sorted_set.Score(s),
				})
			}
		case float64:
			s, _ := score.(float64)
			members = append(members, sorted_set.MemberParam{
				Value: sorted_set.Value(cmd[membersStartIndex:][i+1]),
				Score: sorted_set.Score(s),
			})
		case int:
			s, _ := score.(int)
			members = append(members, sorted_set.MemberParam{
				Value: sorted_set.Value(cmd[membersStartIndex:][i+1]),
				Score: sorted_set.Score(s),
			})
		}
	}

	// Parse options using membersStartIndex as the upper limit
	if membersStartIndex > 2 {
		options := cmd[2:membersStartIndex]
		for _, option := range options {
			if slices.Contains([]string{"xx", "nx"}, strings.ToLower(option)) {
				updatePolicy = option
				// If option is "NX" and comparison is not nil, return an error
				if strings.EqualFold(option, "NX") && comparison != nil {
					return nil, errors.New("GT/LT flags not allowed if NX flag is provided")
				}
				continue
			}
			if slices.Contains([]string{"gt", "lt"}, strings.ToLower(option)) {
				comparison = option
				// If updatePolicy is "NX", return an error
				up, _ := updatePolicy.(string)
				if strings.EqualFold(up, "NX") {
					return nil, errors.New("GT/LT flags not allowed if NX flag is provided")
				}
				continue
			}
			if strings.EqualFold(option, "ch") {
				changed = option
				continue
			}
			if strings.EqualFold(option, "incr") {
				incr = option
				// If members length is more than 1, return an error
				if len(members) > 1 {
					return nil, errors.New("cannot pass more than one score/member pair when INCR flag is provided")
				}
				continue
			}
			return nil, fmt.Errorf("invalid option %s", option)
		}
	}

	if server.KeyExists(ctx, key) {
		// Key exists
		_, err = server.KeyLock(ctx, key)
		if err != nil {
			return nil, err
		}
		defer server.KeyUnlock(ctx, key)
		set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", key)
		}
		count, err := set.AddOrUpdate(members, updatePolicy, comparison, changed, incr)
		if err != nil {
			return nil, err
		}
		// If INCR option is provided, return the new score value
		if incr != nil {
			m := set.Get(members[0].Value)
			return []byte(fmt.Sprintf("+%f\r\n", m.Score)), nil
		}

		return []byte(fmt.Sprintf(":%d\r\n", count)), nil
	}

	// Key does not exist
	if _, err = server.CreateKeyAndLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	set := sorted_set.NewSortedSet(members)
	if err = server.SetValue(ctx, key, set); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", set.Cardinality())), nil
}

func handleZCARD(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zcardKeyFunc(cmd)
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

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	return []byte(fmt.Sprintf(":%d\r\n", set.Cardinality())), nil
}

func handleZCOUNT(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zcountKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]

	minimum := sorted_set.Score(math.Inf(-1))
	switch internal.AdaptType(cmd[2]).(type) {
	default:
		return nil, errors.New("min constraint must be a double")
	case string:
		if strings.ToLower(cmd[2]) == "+inf" {
			minimum = sorted_set.Score(math.Inf(1))
		} else {
			return nil, errors.New("min constraint must be a double")
		}
	case float64:
		s, _ := internal.AdaptType(cmd[2]).(float64)
		minimum = sorted_set.Score(s)
	case int:
		s, _ := internal.AdaptType(cmd[2]).(int)
		minimum = sorted_set.Score(s)
	}

	maximum := sorted_set.Score(math.Inf(1))
	switch internal.AdaptType(cmd[3]).(type) {
	default:
		return nil, errors.New("max constraint must be a double")
	case string:
		if strings.ToLower(cmd[3]) == "-inf" {
			maximum = sorted_set.Score(math.Inf(-1))
		} else {
			return nil, errors.New("max constraint must be a double")
		}
	case float64:
		s, _ := internal.AdaptType(cmd[3]).(float64)
		maximum = sorted_set.Score(s)
	case int:
		s, _ := internal.AdaptType(cmd[3]).(int)
		maximum = sorted_set.Score(s)
	}

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	var members []sorted_set.MemberParam
	for _, m := range set.GetAll() {
		if m.Score >= minimum && m.Score <= maximum {
			members = append(members, m)
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", len(members))), nil
}

func handleZLEXCOUNT(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zlexcountKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	minimum := cmd[2]
	maximum := cmd[3]

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetAll()

	// Check if all members has the same score
	for i := 0; i < len(members)-2; i++ {
		if members[i].Score != members[i+1].Score {
			return []byte(":0\r\n"), nil
		}
	}

	count := 0

	for _, m := range members {
		if slices.Contains([]int{1, 0}, internal.CompareLex(string(m.Value), minimum)) &&
			slices.Contains([]int{-1, 0}, internal.CompareLex(string(m.Value), maximum)) {
			count += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", count)), nil
}

func handleZDIFF(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zdiffKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	withscoresIndex := slices.IndexFunc(cmd, func(s string) bool {
		return strings.EqualFold(s, "withscores")
	})
	if withscoresIndex > -1 && withscoresIndex < 2 {
		return nil, errors.New(constants.WrongArgsResponse)
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(ctx, key)
			}
		}
	}()

	// Extract base set
	if !server.KeyExists(ctx, keys.ReadKeys[0]) {
		// If base set does not exist, return an empty array
		return []byte("*0\r\n"), nil
	}
	if _, err = server.KeyRLock(ctx, keys.ReadKeys[0]); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, keys.ReadKeys[0])
	baseSortedSet, ok := server.GetValue(ctx, keys.ReadKeys[0]).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", keys.ReadKeys[0])
	}

	// Extract the remaining sets
	var sets []*sorted_set.SortedSet

	for i := 1; i < len(keys.ReadKeys); i++ {
		if !server.KeyExists(ctx, keys.ReadKeys[i]) {
			continue
		}
		locked, err := server.KeyRLock(ctx, keys.ReadKeys[i])
		if err != nil {
			return nil, err
		}
		locks[keys.ReadKeys[i]] = locked
		set, ok := server.GetValue(ctx, keys.ReadKeys[i]).(*sorted_set.SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", keys.ReadKeys[i])
		}
		sets = append(sets, set)
	}

	var diff = baseSortedSet.Subtract(sets)

	res := fmt.Sprintf("*%d", diff.Cardinality())
	includeScores := withscoresIndex != -1 && withscoresIndex >= 2

	for _, m := range diff.GetAll() {
		if includeScores {
			res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
		} else {
			res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZDIFFSTORE(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zdiffstoreKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	destination := keys.WriteKeys[0]

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(ctx, key)
			}
		}
	}()

	// Extract base set
	if !server.KeyExists(ctx, keys.ReadKeys[0]) {
		// If base set does not exist, return 0
		return []byte(":0\r\n"), nil
	}
	if _, err = server.KeyRLock(ctx, keys.ReadKeys[0]); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, keys.ReadKeys[0])
	baseSortedSet, ok := server.GetValue(ctx, keys.ReadKeys[0]).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", keys.ReadKeys[0])
	}

	var sets []*sorted_set.SortedSet

	for i := 1; i < len(keys.ReadKeys); i++ {
		if server.KeyExists(ctx, keys.ReadKeys[i]) {
			if _, err = server.KeyRLock(ctx, keys.ReadKeys[i]); err != nil {
				return nil, err
			}
			set, ok := server.GetValue(ctx, keys.ReadKeys[i]).(*sorted_set.SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", keys.ReadKeys[i])
			}
			sets = append(sets, set)
		}
	}

	diff := baseSortedSet.Subtract(sets)

	if server.KeyExists(ctx, destination) {
		if _, err = server.KeyLock(ctx, destination); err != nil {
			return nil, err
		}
	} else {
		if _, err = server.CreateKeyAndLock(ctx, destination); err != nil {
			return nil, err
		}
	}
	defer server.KeyUnlock(ctx, destination)

	if err = server.SetValue(ctx, destination, diff); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", diff.Cardinality())), nil
}

func handleZINCRBY(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zincrbyKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	member := sorted_set.Value(cmd[3])
	var increment sorted_set.Score

	switch internal.AdaptType(cmd[2]).(type) {
	default:
		return nil, errors.New("increment must be a double")
	case string:
		if strings.EqualFold("-inf", strings.ToLower(cmd[2])) {
			increment = sorted_set.Score(math.Inf(-1))
		} else if strings.EqualFold("+inf", strings.ToLower(cmd[2])) {
			increment = sorted_set.Score(math.Inf(1))
		} else {
			return nil, errors.New("increment must be a double")
		}
	case float64:
		s, _ := internal.AdaptType(cmd[2]).(float64)
		increment = sorted_set.Score(s)
	case int:
		s, _ := internal.AdaptType(cmd[2]).(int)
		increment = sorted_set.Score(s)
	}

	if !server.KeyExists(ctx, key) {
		// If the key does not exist, create a new sorted set at the key with
		// the member and increment as the first value
		if _, err = server.CreateKeyAndLock(ctx, key); err != nil {
			return nil, err
		}
		if err = server.SetValue(
			ctx,
			key,
			sorted_set.NewSortedSet([]sorted_set.MemberParam{{Value: member, Score: increment}}),
		); err != nil {
			return nil, err
		}
		server.KeyUnlock(ctx, key)
		return []byte(fmt.Sprintf("+%s\r\n", strconv.FormatFloat(float64(increment), 'f', -1, 64))), nil
	}

	if _, err = server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)
	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}
	if _, err = set.AddOrUpdate(
		[]sorted_set.MemberParam{
			{Value: member, Score: increment}},
		"xx",
		nil,
		nil,
		"incr"); err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("+%s\r\n",
		strconv.FormatFloat(float64(set.Get(member).Score), 'f', -1, 64))), nil
}

func handleZINTER(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	_, err := zinterKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	keys, weights, aggregate, withscores, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(ctx, key)
			}
		}
	}()

	var setParams []sorted_set.SortedSetParam

	for i := 0; i < len(keys); i++ {
		if !server.KeyExists(ctx, keys[i]) {
			// If any of the keys is non-existent, return an empty array as there's no intersect
			return []byte("*0\r\n"), nil
		}
		if _, err = server.KeyRLock(ctx, keys[i]); err != nil {
			return nil, err
		}
		locks[keys[i]] = true
		set, ok := server.GetValue(ctx, keys[i]).(*sorted_set.SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", keys[i])
		}
		setParams = append(setParams, sorted_set.SortedSetParam{
			Set:    set,
			Weight: weights[i],
		})
	}

	intersect := sorted_set.Intersect(aggregate, setParams...)

	res := fmt.Sprintf("*%d", intersect.Cardinality())

	if intersect.Cardinality() > 0 {
		for _, m := range intersect.GetAll() {
			if withscores {
				res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
			} else {
				res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
			}
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZINTERSTORE(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	k, err := zinterstoreKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	destination := k.WriteKeys[0]

	// Remove the destination keys from the command before parsing it
	cmd = slices.DeleteFunc(cmd, func(s string) bool {
		return s == destination
	})

	keys, weights, aggregate, _, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(ctx, key)
			}
		}
	}()

	var setParams []sorted_set.SortedSetParam

	for i := 0; i < len(keys); i++ {
		if !server.KeyExists(ctx, keys[i]) {
			return []byte(":0\r\n"), nil
		}
		if _, err = server.KeyRLock(ctx, keys[i]); err != nil {
			return nil, err
		}
		locks[keys[i]] = true
		set, ok := server.GetValue(ctx, keys[i]).(*sorted_set.SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", keys[i])
		}
		setParams = append(setParams, sorted_set.SortedSetParam{
			Set:    set,
			Weight: weights[i],
		})
	}

	intersect := sorted_set.Intersect(aggregate, setParams...)

	if server.KeyExists(ctx, destination) && intersect.Cardinality() > 0 {
		if _, err = server.KeyLock(ctx, destination); err != nil {
			return nil, err
		}
	} else if intersect.Cardinality() > 0 {
		if _, err = server.CreateKeyAndLock(ctx, destination); err != nil {
			return nil, err
		}
	}
	defer server.KeyUnlock(ctx, destination)

	if err = server.SetValue(ctx, destination, intersect); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", intersect.Cardinality())), nil
}

func handleZMPOP(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zmpopKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	count := 1
	policy := "min"
	modifierIdx := -1

	// Parse COUNT from command
	countIdx := slices.IndexFunc(cmd, func(s string) bool {
		return strings.ToLower(s) == "count"
	})
	if countIdx != -1 {
		if countIdx < 2 {
			return nil, errors.New(constants.WrongArgsResponse)
		}
		if countIdx == len(cmd)-1 {
			return nil, errors.New("count must be a positive integer")
		}
		c, err := strconv.Atoi(cmd[countIdx+1])
		if err != nil {
			return nil, err
		}
		if c <= 0 {
			return nil, errors.New("count must be a positive integer")
		}
		count = c
		modifierIdx = countIdx
	}

	// Parse MIN/MAX from the command
	policyIdx := slices.IndexFunc(cmd, func(s string) bool {
		return slices.Contains([]string{"min", "max"}, strings.ToLower(s))
	})
	if policyIdx != -1 {
		if policyIdx < 2 {
			return nil, errors.New(constants.WrongArgsResponse)
		}
		policy = strings.ToLower(cmd[policyIdx])
		if modifierIdx == -1 || (policyIdx < modifierIdx) {
			modifierIdx = policyIdx
		}
	}

	for i := 0; i < len(keys.WriteKeys); i++ {
		if server.KeyExists(ctx, keys.WriteKeys[i]) {
			if _, err = server.KeyLock(ctx, keys.WriteKeys[i]); err != nil {
				continue
			}
			v, ok := server.GetValue(ctx, keys.WriteKeys[i]).(*sorted_set.SortedSet)
			if !ok || v.Cardinality() == 0 {
				server.KeyUnlock(ctx, keys.WriteKeys[i])
				continue
			}
			popped, err := v.Pop(count, policy)
			if err != nil {
				server.KeyUnlock(ctx, keys.WriteKeys[i])
				return nil, err
			}
			server.KeyUnlock(ctx, keys.WriteKeys[i])

			res := fmt.Sprintf("*%d", popped.Cardinality())

			for _, m := range popped.GetAll() {
				res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
			}

			res += "\r\n"

			return []byte(res), nil
		}
	}

	return []byte("*0\r\n"), nil
}

func handleZPOP(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zpopKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	count := 1
	policy := "min"

	if strings.EqualFold(cmd[0], "zpopmax") {
		policy = "max"
	}

	if len(cmd) == 3 {
		c, err := strconv.Atoi(cmd[2])
		if err != nil {
			return nil, err
		}
		if c > 0 {
			count = c
		}
	}

	if !server.KeyExists(ctx, key) {
		return []byte("*0\r\n"), nil
	}

	if _, err = server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at key %s is not a sorted set", key)
	}

	popped, err := set.Pop(count, policy)
	if err != nil {
		return nil, err
	}

	res := fmt.Sprintf("*%d", popped.Cardinality())
	for _, m := range popped.GetAll() {
		res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZMSCORE(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zmscoreKeyFunc(cmd)
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

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := cmd[2:]

	res := fmt.Sprintf("*%d", len(members))

	var member sorted_set.MemberObject

	for i := 0; i < len(members); i++ {
		member = set.Get(sorted_set.Value(members[i]))
		if !member.Exists {
			res = fmt.Sprintf("%s\r\n$-1", res)
		} else {
			res = fmt.Sprintf("%s\r\n+%s", res, strconv.FormatFloat(float64(member.Score), 'f', -1, 64))
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZRANDMEMBER(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zrandmemberKeyFunc(cmd)
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
		if c != 0 {
			count = c
		}
	}

	withscores := false
	if len(cmd) == 4 {
		if strings.EqualFold(cmd[3], "withscores") {
			withscores = true
		} else {
			return nil, errors.New("last option must be WITHSCORES")
		}
	}

	if !server.KeyExists(ctx, key) {
		return []byte("$-1\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetRandom(count)

	res := fmt.Sprintf("*%d", len(members))
	for _, m := range members {
		if withscores {
			res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
		} else {
			res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZRANK(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zrankKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	member := cmd[2]
	withscores := false

	if len(cmd) == 4 && strings.EqualFold(cmd[3], "withscores") {
		withscores = true
	}

	if !server.KeyExists(ctx, key) {
		return []byte("$-1\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetAll()
	slices.SortFunc(members, func(a, b sorted_set.MemberParam) int {
		if strings.EqualFold(cmd[0], "zrevrank") {
			return cmp.Compare(b.Score, a.Score)
		}
		return cmp.Compare(a.Score, b.Score)
	})

	for i := 0; i < len(members); i++ {
		if members[i].Value == sorted_set.Value(member) {
			if withscores {
				score := strconv.FormatFloat(float64(members[i].Score), 'f', -1, 64)
				return []byte(fmt.Sprintf("*2\r\n:%d\r\n$%d\r\n%s\r\n", i, len(score), score)), nil
			} else {
				return []byte(fmt.Sprintf("*1\r\n:%d\r\n", i)), nil
			}
		}
	}

	return []byte("$-1\r\n"), nil
}

func handleZREM(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zremKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	deletedCount := 0
	for _, m := range cmd[2:] {
		if set.Remove(sorted_set.Value(m)) {
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

func handleZSCORE(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zscoreKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]

	if !server.KeyExists(ctx, key) {
		return []byte("$-1\r\n"), nil
	}
	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)
	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}
	member := set.Get(sorted_set.Value(cmd[2]))
	if !member.Exists {
		return []byte("$-1\r\n"), nil
	}

	score := strconv.FormatFloat(float64(member.Score), 'f', -1, 64)

	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(score), score)), nil
}

func handleZREMRANGEBYSCORE(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zremrangebyscoreKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]

	deletedCount := 0

	minimum, err := strconv.ParseFloat(cmd[2], 64)
	if err != nil {
		return nil, err
	}

	maximum, err := strconv.ParseFloat(cmd[3], 64)
	if err != nil {
		return nil, err
	}

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	for _, m := range set.GetAll() {
		if m.Score >= sorted_set.Score(minimum) && m.Score <= sorted_set.Score(maximum) {
			set.Remove(m.Value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

func handleZREMRANGEBYRANK(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zremrangebyrankKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]

	start, err := strconv.Atoi(cmd[2])
	if err != nil {
		return nil, err
	}

	stop, err := strconv.Atoi(cmd[3])
	if err != nil {
		return nil, err
	}

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	if start < 0 {
		start = start + set.Cardinality()
	}
	if stop < 0 {
		stop = stop + set.Cardinality()
	}

	if start < 0 || start > set.Cardinality()-1 || stop < 0 || stop > set.Cardinality()-1 {
		return nil, errors.New("indices out of bounds")
	}

	members := set.GetAll()
	slices.SortFunc(members, func(a, b sorted_set.MemberParam) int {
		return cmp.Compare(a.Score, b.Score)
	})

	deletedCount := 0

	if start < stop {
		for i := start; i <= stop; i++ {
			set.Remove(members[i].Value)
			deletedCount += 1
		}
	} else {
		for i := stop; i <= start; i++ {
			set.Remove(members[i].Value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

func handleZREMRANGEBYLEX(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zremrangebylexKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.WriteKeys[0]
	minimum := cmd[2]
	maximum := cmd[3]

	if !server.KeyExists(ctx, key) {
		return []byte(":0\r\n"), nil
	}

	if _, err = server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetAll()

	// Check if all the members have the same score. If not, return 0
	for i := 0; i < len(members)-1; i++ {
		if members[i].Score != members[i+1].Score {
			return []byte(":0\r\n"), nil
		}
	}

	deletedCount := 0

	// All the members have the same score
	for _, m := range members {
		if slices.Contains([]int{1, 0}, internal.CompareLex(string(m.Value), minimum)) &&
			slices.Contains([]int{-1, 0}, internal.CompareLex(string(m.Value), maximum)) {
			set.Remove(m.Value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n", deletedCount)), nil
}

func handleZRANGE(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zrangeKeyCount(cmd)
	if err != nil {
		return nil, err
	}

	key := keys.ReadKeys[0]
	policy := "byscore"
	scoreStart := math.Inf(-1) // Lower bound if policy is "byscore"
	scoreStop := math.Inf(1)   // Upper bound if policy is "byscore"
	lexStart := cmd[2]         // Lower bound if policy is "bylex"
	lexStop := cmd[3]          // Upper bound if policy is "bylex"
	offset := 0
	count := -1

	withscores := slices.ContainsFunc(cmd[4:], func(s string) bool {
		return strings.EqualFold(s, "withscores")
	})

	reverse := slices.ContainsFunc(cmd[4:], func(s string) bool {
		return strings.EqualFold(s, "rev")
	})

	if slices.ContainsFunc(cmd[4:], func(s string) bool {
		return strings.EqualFold(s, "bylex")
	}) {
		policy = "bylex"
	} else {
		// policy is "byscore" make sure start and stop are valid float values
		scoreStart, err = strconv.ParseFloat(cmd[2], 64)
		if err != nil {
			return nil, err
		}
		scoreStop, err = strconv.ParseFloat(cmd[3], 64)
		if err != nil {
			return nil, err
		}
	}

	if slices.ContainsFunc(cmd[4:], func(s string) bool {
		return strings.EqualFold(s, "limit")
	}) {
		limitIdx := slices.IndexFunc(cmd[4:], func(s string) bool {
			return strings.EqualFold(s, "limit")
		})
		if limitIdx != -1 && limitIdx > len(cmd[4:])-3 {
			return nil, errors.New("limit should contain offset and count as integers")
		}
		offset, err = strconv.Atoi(cmd[4:][limitIdx+1])
		if err != nil {
			return nil, errors.New("limit offset must be integer")
		}
		if offset < 0 {
			return nil, errors.New("limit offset must be >= 0")
		}
		count, err = strconv.Atoi(cmd[4:][limitIdx+2])
		if err != nil {
			return nil, errors.New("limit count must be integer")
		}
	}

	if !server.KeyExists(ctx, key) {
		return []byte("*0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, key)

	set, ok := server.GetValue(ctx, key).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	if offset > set.Cardinality() {
		return []byte("*0\r\n"), nil
	}
	if count < 0 {
		count = set.Cardinality() - offset
	}

	members := set.GetAll()
	if strings.EqualFold(policy, "byscore") {
		slices.SortFunc(members, func(a, b sorted_set.MemberParam) int {
			// Do a score sort
			if reverse {
				return cmp.Compare(b.Score, a.Score)
			}
			return cmp.Compare(a.Score, b.Score)
		})
	}
	if strings.EqualFold(policy, "bylex") {
		// If policy is BYLEX, all the elements must have the same score
		for i := 0; i < len(members)-1; i++ {
			if members[i].Score != members[i+1].Score {
				return []byte("*0\r\n"), nil
			}
		}
		slices.SortFunc(members, func(a, b sorted_set.MemberParam) int {
			if reverse {
				return internal.CompareLex(string(b.Value), string(a.Value))
			}
			return internal.CompareLex(string(a.Value), string(b.Value))
		})
	}

	var resultMembers []sorted_set.MemberParam

	for i := offset; i <= count; i++ {
		if i >= len(members) {
			break
		}
		if strings.EqualFold(policy, "byscore") {
			if members[i].Score >= sorted_set.Score(scoreStart) && members[i].Score <= sorted_set.Score(scoreStop) {
				resultMembers = append(resultMembers, members[i])
			}
			continue
		}
		if slices.Contains([]int{1, 0}, internal.CompareLex(string(members[i].Value), lexStart)) &&
			slices.Contains([]int{-1, 0}, internal.CompareLex(string(members[i].Value), lexStop)) {
			resultMembers = append(resultMembers, members[i])
		}
	}

	res := fmt.Sprintf("*%d", len(resultMembers))

	for _, m := range resultMembers {
		if withscores {
			res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
		} else {
			res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZRANGESTORE(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	keys, err := zrangeStoreKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	destination := keys.WriteKeys[0]
	source := keys.ReadKeys[0]
	policy := "byscore"
	scoreStart := math.Inf(-1) // Lower bound if policy is "byscore"
	scoreStop := math.Inf(1)   // Upper bound if policy is "byfloat"
	lexStart := cmd[3]         // Lower bound if policy is "bylex"
	lexStop := cmd[4]          // Upper bound if policy is "bylex"
	offset := 0
	count := -1

	reverse := slices.ContainsFunc(cmd[5:], func(s string) bool {
		return strings.EqualFold(s, "rev")
	})

	if slices.ContainsFunc(cmd[5:], func(s string) bool {
		return strings.EqualFold(s, "bylex")
	}) {
		policy = "bylex"
	} else {
		// policy is "byscore" make sure start and stop are valid float values
		scoreStart, err = strconv.ParseFloat(cmd[3], 64)
		if err != nil {
			return nil, err
		}
		scoreStop, err = strconv.ParseFloat(cmd[4], 64)
		if err != nil {
			return nil, err
		}
	}

	if slices.ContainsFunc(cmd[5:], func(s string) bool {
		return strings.EqualFold(s, "limit")
	}) {
		limitIdx := slices.IndexFunc(cmd[5:], func(s string) bool {
			return strings.EqualFold(s, "limit")
		})
		if limitIdx != -1 && limitIdx > len(cmd[5:])-3 {
			return nil, errors.New("limit should contain offset and count as integers")
		}
		offset, err = strconv.Atoi(cmd[5:][limitIdx+1])
		if err != nil {
			return nil, errors.New("limit offset must be integer")
		}
		if offset < 0 {
			return nil, errors.New("limit offset must be >= 0")
		}
		count, err = strconv.Atoi(cmd[5:][limitIdx+2])
		if err != nil {
			return nil, errors.New("limit count must be integer")
		}
	}

	if !server.KeyExists(ctx, source) {
		return []byte("*0\r\n"), nil
	}

	if _, err = server.KeyRLock(ctx, source); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(ctx, source)

	set, ok := server.GetValue(ctx, source).(*sorted_set.SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", source)
	}

	if offset > set.Cardinality() {
		return []byte(":0\r\n"), nil
	}
	if count < 0 {
		count = set.Cardinality() - offset
	}

	members := set.GetAll()
	if strings.EqualFold(policy, "byscore") {
		slices.SortFunc(members, func(a, b sorted_set.MemberParam) int {
			// Do a score sort
			if reverse {
				return cmp.Compare(b.Score, a.Score)
			}
			return cmp.Compare(a.Score, b.Score)
		})
	}
	if strings.EqualFold(policy, "bylex") {
		// If policy is BYLEX, all the elements must have the same score
		for i := 0; i < len(members)-1; i++ {
			if members[i].Score != members[i+1].Score {
				return []byte(":0\r\n"), nil
			}
		}
		slices.SortFunc(members, func(a, b sorted_set.MemberParam) int {
			if reverse {
				return internal.CompareLex(string(b.Value), string(a.Value))
			}
			return internal.CompareLex(string(a.Value), string(b.Value))
		})
	}

	var resultMembers []sorted_set.MemberParam

	for i := offset; i <= count; i++ {
		if i >= len(members) {
			break
		}
		if strings.EqualFold(policy, "byscore") {
			if members[i].Score >= sorted_set.Score(scoreStart) && members[i].Score <= sorted_set.Score(scoreStop) {
				resultMembers = append(resultMembers, members[i])
			}
			continue
		}
		if slices.Contains([]int{1, 0}, internal.CompareLex(string(members[i].Value), lexStart)) &&
			slices.Contains([]int{-1, 0}, internal.CompareLex(string(members[i].Value), lexStop)) {
			resultMembers = append(resultMembers, members[i])
		}
	}

	newSortedSet := sorted_set.NewSortedSet(resultMembers)

	if server.KeyExists(ctx, destination) {
		if _, err = server.KeyLock(ctx, destination); err != nil {
			return nil, err
		}
	} else {
		if _, err = server.CreateKeyAndLock(ctx, destination); err != nil {
			return nil, err
		}
	}
	defer server.KeyUnlock(ctx, destination)

	if err = server.SetValue(ctx, destination, newSortedSet); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", newSortedSet.Cardinality())), nil
}

func handleZUNION(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	if _, err := zunionKeyFunc(cmd); err != nil {
		return nil, err
	}

	keys, weights, aggregate, withscores, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(ctx, key)
			}
		}
	}()

	var setParams []sorted_set.SortedSetParam

	for i := 0; i < len(keys); i++ {
		if server.KeyExists(ctx, keys[i]) {
			if _, err = server.KeyRLock(ctx, keys[i]); err != nil {
				return nil, err
			}
			locks[keys[i]] = true
			set, ok := server.GetValue(ctx, keys[i]).(*sorted_set.SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", keys[i])
			}
			setParams = append(setParams, sorted_set.SortedSetParam{
				Set:    set,
				Weight: weights[i],
			})
		}
	}

	union := sorted_set.Union(aggregate, setParams...)

	res := fmt.Sprintf("*%d", union.Cardinality())
	for _, m := range union.GetAll() {
		if withscores {
			res += fmt.Sprintf("\r\n*2\r\n$%d\r\n%s\r\n+%s", len(m.Value), m.Value, strconv.FormatFloat(float64(m.Score), 'f', -1, 64))
		} else {
			res += fmt.Sprintf("\r\n*1\r\n$%d\r\n%s", len(m.Value), m.Value)
		}
	}

	res += "\r\n"

	return []byte(res), nil
}

func handleZUNIONSTORE(ctx context.Context, cmd []string, server types.EchoVault, _ *net.Conn) ([]byte, error) {
	k, err := zunionstoreKeyFunc(cmd)
	if err != nil {
		return nil, err
	}

	destination := k.WriteKeys[0]

	// Remove destination key from list of keys
	cmd = slices.DeleteFunc(cmd, func(s string) bool {
		return s == destination
	})

	keys, weights, aggregate, _, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(ctx, key)
			}
		}
	}()

	var setParams []sorted_set.SortedSetParam

	for i := 0; i < len(keys); i++ {
		if server.KeyExists(ctx, keys[i]) {
			if _, err = server.KeyRLock(ctx, keys[i]); err != nil {
				return nil, err
			}
			locks[keys[i]] = true
			set, ok := server.GetValue(ctx, keys[i]).(*sorted_set.SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", keys[i])
			}
			setParams = append(setParams, sorted_set.SortedSetParam{
				Set:    set,
				Weight: weights[i],
			})
		}
	}

	union := sorted_set.Union(aggregate, setParams...)

	if server.KeyExists(ctx, destination) {
		if _, err = server.KeyLock(ctx, destination); err != nil {
			return nil, err
		}
	} else {
		if _, err = server.CreateKeyAndLock(ctx, destination); err != nil {
			return nil, err
		}
	}
	defer server.KeyUnlock(ctx, destination)

	if err = server.SetValue(ctx, destination, union); err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(":%d\r\n", union.Cardinality())), nil
}

func Commands() []types.Command {
	return []types.Command{
		{
			Command:    "zadd",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(ZADD key [NX | XX] [GT | LT] [CH] [INCR] score member [score member...])
Adds all the specified members with the specified scores to the sorted set at the key.
"NX" only adds the member if it currently does not exist in the sorted set.
"XX" only updates the scores of members that exist in the sorted set.
"GT"" only updates the score if the new score is greater than the current score.
"LT" only updates the score if the new score is less than the current score.
"CH" modifies the result to return total number of members changed + added, instead of only new members added.
"INCR" modifies the command to act like ZINCRBY, only one score/member pair can be specified in this mode.`,
			Sync:              true,
			KeyExtractionFunc: zaddKeyFunc,
			HandlerFunc:       handleZADD,
		},
		{
			Command:    "zcard",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZCARD key) Returns the set cardinality of the sorted set at key.
If the key does not exist, 0 is returned, otherwise the cardinality of the sorted set is returned.
If the key holds a value that is not a sorted set, this command will return an error.`,
			Sync:              false,
			KeyExtractionFunc: zcardKeyFunc,
			HandlerFunc:       handleZCARD,
		},
		{
			Command:    "zcount",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZCOUNT key min max) 
Returns the number of elements in the sorted set key with scores in the range of min and max.
If the key does not exist, a count of 0 is returned, otherwise return the count.
If the key holds a value that is not a sorted set, an error is returned.`,
			Sync:              false,
			KeyExtractionFunc: zcountKeyFunc,
			HandlerFunc:       handleZCOUNT,
		},
		{
			Command:    "zdiff",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZDIFF key [key...] [WITHSCORES]) 
Computes the difference between all the sorted sets specified in the list of keys and returns the result.`,
			Sync:              false,
			KeyExtractionFunc: zdiffKeyFunc,
			HandlerFunc:       handleZDIFF,
		},
		{
			Command:    "zdiffstore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZDIFFSTORE destination key [key...]). 
Computes the difference between all the sorted sets specifies in the list of keys. Stores the result in destination.
If the base set (first key) does not exist, return 0, otherwise, return the cardinality of the diff.`,
			Sync:              true,
			KeyExtractionFunc: zdiffstoreKeyFunc,
			HandlerFunc:       handleZDIFFSTORE,
		},
		{
			Command:    "zincrby",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(ZINCRBY key increment member). 
Increments the score of the specified sorted set's member by the increment. If the member does not exist, it is created.
If the key does not exist, it is created with new sorted set and the member added with the increment as its score.`,
			Sync:              true,
			KeyExtractionFunc: zincrbyKeyFunc,
			HandlerFunc:       handleZINCRBY,
		},
		{
			Command:    "zinter",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZINTER key [key ...] [WEIGHTS weight [weight ...]] [AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]).
Computes the intersection of the sets in the keys, with weights, aggregate and scores`,
			Sync:              false,
			KeyExtractionFunc: zinterKeyFunc,
			HandlerFunc:       handleZINTER,
		},
		{
			Command:    "zinterstore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `
(ZINTERSTORE destination key [key ...] [WEIGHTS weight [weight ...]] [AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]).
Computes the intersection of the sets in the keys, with weights, aggregate and scores. The result is stored in destination.`,
			Sync:              true,
			KeyExtractionFunc: zinterstoreKeyFunc,
			HandlerFunc:       handleZINTERSTORE,
		},
		{
			Command:    "zmpop",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZMPOP key [key ...] <MIN | MAX> [COUNT count])
Pop a 'count' elements from multiple sorted sets. MIN or MAX determines whether to pop elements with the lowest or highest scores
respectively.`,
			Sync:              true,
			KeyExtractionFunc: zmpopKeyFunc,
			HandlerFunc:       handleZMPOP,
		},
		{
			Command:    "zmscore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.FastCategory},
			Description: `(ZMSCORE key member [member ...])
Returns the associated scores of the specified member in the sorted set. 
Returns nil for members that do not exist in the set`,
			Sync:              false,
			KeyExtractionFunc: zmscoreKeyFunc,
			HandlerFunc:       handleZMSCORE,
		},
		{
			Command:    "zpopmax",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZPOPMAX key [count])
Removes and returns 'count' number of members in the sorted set with the highest scores. Default count is 1.`,
			Sync:              true,
			KeyExtractionFunc: zpopKeyFunc,
			HandlerFunc:       handleZPOP,
		},
		{
			Command:    "zpopmin",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZPOPMIN key [count])
Removes and returns 'count' number of members in the sorted set with the lowest scores. Default count is 1.`,
			Sync:              true,
			KeyExtractionFunc: zpopKeyFunc,
			HandlerFunc:       handleZPOP,
		},
		{
			Command:    "zrandmember",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZRANDMEMBER key [count [WITHSCORES]])
Return a list of length equivalent to count containing random members of the sorted set.
If count is negative, repeated elements are allowed. If count is positive, the returned elements will be distinct.
WITHSCORES modifies the result to include scores in the result.`,
			Sync:              false,
			KeyExtractionFunc: zrandmemberKeyFunc,
			HandlerFunc:       handleZRANDMEMBER,
		},
		{
			Command:    "zrank",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZRANK key member [WITHSCORE])
Returns the rank of the specified member in the sorted set. WITHSCORE modifies the result to also return the score.`,
			Sync:              false,
			KeyExtractionFunc: zrankKeyFunc,
			HandlerFunc:       handleZRANK,
		},
		{
			Command:    "zrevrank",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZREVRANK key member [WITHSCORE])
Returns the rank of the member in the sorted set in reverse order. 
WITHSCORE modifies the result to include the score.`,
			Sync:              false,
			KeyExtractionFunc: zrevrankKeyFunc,
			HandlerFunc:       handleZRANK,
		},
		{
			Command:    "zrem",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.FastCategory},
			Description: `(ZREM key member [member ...]) Removes the listed members from the sorted set.
Returns the number of elements removed.`,
			Sync:              true,
			KeyExtractionFunc: zremKeyFunc,
			HandlerFunc:       handleZREM,
		},
		{
			Command:           "zscore",
			Module:            constants.SortedSetModule,
			Categories:        []string{constants.SortedSetCategory, constants.ReadCategory, constants.FastCategory},
			Description:       `(ZSCORE key member) Returns the score of the member in the sorted set.`,
			Sync:              false,
			KeyExtractionFunc: zscoreKeyFunc,
			HandlerFunc:       handleZSCORE,
		},
		{
			Command:           "zremrangebylex",
			Module:            constants.SortedSetModule,
			Categories:        []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description:       `(ZREMRANGEBYLEX key min max) Removes the elements in the lexicographical range between min and max`,
			Sync:              true,
			KeyExtractionFunc: zremrangebylexKeyFunc,
			HandlerFunc:       handleZREMRANGEBYLEX,
		},
		{
			Command:    "zremrangebyrank",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZREMRANGEBYRANK key start stop) Removes the elements in the rank range between start and stop.
The elements are ordered from lowest score to highest score`,
			Sync:              true,
			KeyExtractionFunc: zremrangebyrankKeyFunc,
			HandlerFunc:       handleZREMRANGEBYRANK,
		},
		{
			Command:           "zremrangebyscore",
			Module:            constants.SortedSetModule,
			Categories:        []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description:       `(ZREMRANGEBYSCORE key min max) Removes the elements whose scores are in the range between min and max`,
			Sync:              true,
			KeyExtractionFunc: zremrangebyscoreKeyFunc,
			HandlerFunc:       handleZREMRANGEBYSCORE,
		},
		{
			Command:    "zlexcount",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZLEXCOUNT key min max) Returns the number of elements in within the sorted set within the 
lexicographical range between min and max. Returns 0, if the keys does not exist or if all the members do not have
the same score. If the value held at key is not a sorted set, an error is returned`,
			Sync:              false,
			KeyExtractionFunc: zlexcountKeyFunc,
			HandlerFunc:       handleZLEXCOUNT,
		},
		{
			Command:    "zrange",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZRANGE key start stop [BYSCORE | BYLEX] [REV] [LIMIT offset count]
  [WITHSCORES]) Returns the range of elements in the sorted set`,
			Sync:              false,
			KeyExtractionFunc: zrangeKeyCount,
			HandlerFunc:       handleZRANGE,
		},
		{
			Command:    "zrangestore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `ZRANGESTORE destination source start stop [BYSCORE | BYLEX] [REV] [LIMIT offset count]
  [WITHSCORES] Retrieve the range of elements in the sorted set and store it in destination`,
			Sync:              true,
			KeyExtractionFunc: zrangeStoreKeyFunc,
			HandlerFunc:       handleZRANGESTORE,
		},
		{
			Command:    "zunion",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.ReadCategory, constants.SlowCategory},
			Description: `(ZUNION key [key ...] [WEIGHTS weight [weight ...]]
[AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]) Return the union of the sorted sets in keys. The scores of each member of 
a sorted set are multiplied by the corresponding weight in WEIGHTS. Aggregate determines how the scores are combined.
WITHSCORES option determines whether to return the result with scores included.`,
			Sync:              false,
			KeyExtractionFunc: zunionKeyFunc,
			HandlerFunc:       handleZUNION,
		},
		{
			Command:    "zunionstore",
			Module:     constants.SortedSetModule,
			Categories: []string{constants.SortedSetCategory, constants.WriteCategory, constants.SlowCategory},
			Description: `(ZUNIONSTORE destination key [key ...] [WEIGHTS weight [weight ...]]
[AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]) Return the union of the sorted sets in keys. The scores of each member of 
a sorted set are multiplied by the corresponding weight in WEIGHTS. Aggregate determines how the scores are combined.
The resulting union is stored at the destination key.`,
			Sync:              true,
			KeyExtractionFunc: zunionstoreKeyFunc,
			HandlerFunc:       handleZUNIONSTORE,
		},
	}
}
