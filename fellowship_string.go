// Code generated by "strunger -type=Fellowship"; DO NOT EDIT.

package main

import "strconv"
import "strings"

const _Fellowship_name = "AAAlAnonGuest"

var _Fellowship_index = [...]uint8{0, 2, 8, 13}

func (i Fellowship) String() string {
	i -= 1
	if i < 0 || i >= Fellowship(len(_Fellowship_index)-1) {
		return "Fellowship(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	foo := _Fellowship_name[_Fellowship_index[i]:_Fellowship_index[i+1]]
	return strings.Replace(foo, "_", " ", -1)
}
