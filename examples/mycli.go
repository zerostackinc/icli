// Copyright 2015 ZeroStack, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// This file contains the source for the example interactive cli built using
// icli. An example command "hello" is registered in the list of commands.
// For interactive test run it as:
// %go run mycli.go
// For non-interactive mode:
// %go run mycli.go hello --name zerostack

package main

import (
  "fmt"
  "os"
  "path"

  "github.com/zerostackinc/icli"
  "gopkg.in/ukautz/clif.v0"
)

const (
  cPrompt      = "mycli>"
  cVersion     = "0.1.0"
  cHistoryFile = ".mycli_history"
)

// mycli embeds ICli.
type mycli struct {
  *icli.ICli
}

var cli mycli

// commands describes the list of Commands in mycli. These are built as
// functions on the cli object so the registered set/unset option values can be
// accessed in the command.
var commands = []*clif.Command{
  clif.NewCommand("hello", "greeting message", cli.helloCommand).
    NewOption("name", "n", "name", "", false, false),
}

// hello greets back with the name option
func (c *mycli) helloCommand(cmd *clif.Command) {
  name := c.GetOption(cmd, "name")
  if name == "" {
    fmt.Printf("please provide a name using the --name flag or set command\n")
    return
  }
  fmt.Printf("Hello %s\n", name)
}

func main() {
  var err error

  fname := path.Join(os.Getenv("HOME"), cHistoryFile)

  cli.ICli, err = icli.NewICli("mycli", cPrompt, cVersion, fname, "My CLI")
  if err != nil {
    fmt.Printf("could not start mycli :: %v", err)
    return
  }

  cli.AddCommands(commands)
  cli.Start(os.Args[1:])
}
