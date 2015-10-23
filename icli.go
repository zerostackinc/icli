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
// icli is a package to build interactive CLIs.
//
// icli combines liner and clif to provide an interactive shell which supports
// user defined commands and line editing with history and search. List of
// keyboard special keys supported on the shell can be found in the README for
// liner. History is stored in the input history file specified in NewICli.
// Built-in commands(functionality) include help, list, quit, set and unset.
// If the icli is started with more than just the command then it uses the
// args to run the command in a non-interactive mode.

package icli

import (
  "fmt"
  "os"
  "reflect"
  "regexp"
  "strings"

  "github.com/peterh/liner"
  "gopkg.in/ukautz/clif.v0"
)

var (
  // ErrQuit is returned when user types quit/exit
  ErrQuit = fmt.Errorf("goodbye")
  // ErrUnknown is returned when user types unknown command
  ErrUnknown = fmt.Errorf("unknown command")
)

// ICli is an interactive CLI package.
type ICli struct {
  *clif.Cli

  term       *liner.State
  prompt     string
  history    string
  optionList map[string]interface{}
  optionSet  map[string]string
}

// NewICli creates a new ICli object and initializes it.
func NewICli(name, prompt, version, history, desc string) (*ICli, error) {
  // Initialize the clif interfaces
  //clif.DefaultStyles = clif.SunburnStyles
  icli := &ICli{
    Cli:       clif.New(name, version, desc),
    term:      liner.NewLiner(),
    prompt:    prompt,
    history:   history,
    optionSet: make(map[string]string),
  }

  f, err := os.Open(history)
  if err != nil {
    f, err = os.Create(history)
    if err != nil {
      return nil, fmt.Errorf("could not open or create history file %s :: %v",
        history, err)
    }
  }
  defer f.Close()
  _, err = icli.term.ReadHistory(f)
  if err != nil {
    return nil, fmt.Errorf("could not read history file %s :: %v", history, err)
  }

  // set ctrl-c to abort the input.
  icli.term.SetCtrlCAborts(true)
  return icli, nil
}

// SetGlobalOption sets a key/value pair of global options that is queried
// when GetOption is called.
func (i *ICli) SetGlobalOption(key, value string) error {
  _, ok := i.optionList[key]
  if !ok {
    return fmt.Errorf("unknown option %s", key)
  }
  i.optionSet[key] = value
  return nil
}

// UnsetGlobalOption removes global option from ICli.
// Ignores errors when unset is called on a not previously set option.
func (i *ICli) UnsetGlobalOption(key string) error {
  delete(i.optionSet, key)
  return nil
}

// GetGlobalOptions prints all the global options into a string.
func (i *ICli) GetGlobalOptions() string {
  opt := ""
  for key, val := range i.optionSet {
    opt += fmt.Sprintf("%s=%s\n", key, val)
  }
  return opt
}

// GetOption looks for option in the cmd args. If it does not exist then it
// looks for it in the global options.
func (i *ICli) GetOption(cmd *clif.Command, name string) string {
  if i == nil || cmd == nil || name == "" {
    return ""
  }
  opt := cmd.Option(name)
  if opt != nil && opt.Provided() {
    return opt.String()
  }
  global, ok := i.optionSet[name]
  if ok {
    return global
  }
  return ""
}

// AddCommands adds the input commands to the list of commands in icli.
func (i *ICli) AddCommands(commands []*clif.Command) error {
  for _, cb := range commands {
    i.Add(cb)
  }
  i.updateCompleter()
  i.updateOptionList()
  return nil
}

// RunCommand runs one command after parsing arguments. Since we are running
// this as an interactive loop, we skip a lot of initialization that is in
// clif.RunWith (which panics on trying to re-init)
func (i *ICli) RunCommand(args []string) error {
  if args == nil {
    args = []string{}
  }
  // determine command or print out default
  name := ""
  cargs := []string{}
  if len(args) < 1 ||
    (i.DefaultCommand == "list" && len(args) == 1 &&
      (args[0] == "-h" || args[0] == "--help")) {
    name = i.DefaultCommand
  } else if len(args) == 1 && (args[0] == "quit" || args[0] == "exit") {
    return ErrQuit
  } else if len(args) > 0 && (args[0] == "set") {
    i.setCommand(args[1:])
    return nil
  } else if len(args) > 0 && (args[0] == "unset") {
    i.unsetCommand(args[1:])
    return nil
  } else {
    name = args[0]
    cargs = args[1:]
  }

  if c, ok := i.Commands[name]; ok {
    defer func() {
      // NOTE(kiran): since the same command object is reused we reset the
      // Options and Arguments values which will be parsed again from input
      // line on next command.
      for k := range c.Arguments {
        c.Arguments[k].Values = nil
      }
      for k := range c.Options {
        c.Options[k].Values = nil
      }
    }()
    // parse arguments & options
    err := c.Parse(cargs)
    if help := c.Option("help"); help != nil && help.Bool() {
      i.Output().Printf(clif.DescribeCommand(c))
      return nil
    }
    if err != nil {
      i.Output().Printf("parse error: %s\n", err)
      i.Output().Printf(clif.DescribeCommand(c))
      return err
    }
    // execute callback for function and handle result
    if res, err := i.Call(c); err != nil {
      i.Output().Printf(err.Error())
    } else {
      errType := reflect.TypeOf((*error)(nil)).Elem()
      if len(res) > 0 && res[0].Type().Implements(errType) && !res[0].IsNil() {
        i.Output().Printf("failure in execution: %s\n",
          res[0].Interface().(error))
      }
      return err
    }
  } else {
    i.Output().Printf("error: command \"%s\" unknown\n", args[0])
    return ErrUnknown
  }
  return nil
}

// Start starts the console read loop and executes the commands.
func (i *ICli) Start(args []string) error {
  defer i.term.Close()

  // If we get cmdline args then we do a one time execution and quit instead
  // of starting a prompt.
  if len(args) > 0 {
    return i.RunCommand(args)
  }

  if f, err := os.Open(i.history); err == nil {
    i.term.ReadHistory(f)
    f.Close()
  }

  var (
    cmd string
    err error
  )

  for {
    // read one line at a time.
    if cmd, err = i.term.Prompt(i.prompt); err != nil {
      // On ^C just continue instead of quitting.
      if err == liner.ErrPromptAborted {
        i.Output().Printf("type \"quit\" to quit\n")
        continue
      } else {
        i.Output().Printf("quitting on error :: %v", err)
        break
      }
    }
    if cmd == "" {
      continue
    }
    // we have to handle quotes around args etc.
    // TODO(kiran): this might need to become a full parser instead of regexp
    r := regexp.MustCompile("'.+'|\".+\"|\\S+")
    args := r.FindAllString(cmd, -1)
    err := i.RunCommand(args)
    // We only stop on ErrQuit
    if err == ErrQuit {
      break
    }
    i.term.AppendHistory(cmd)
  }

  if f, err := os.Create(i.history); err != nil {
    i.Output().Printf("error writing history file: %v\n", err)
  } else {
    i.term.WriteHistory(f)
    f.Close()
  }
  return nil
}

//////////////////////////////////////////////////////////////////////////////

// updateCompleter will update the cmd completer by walking through all
// commands in the list.
func (i *ICli) updateCompleter() error {
  // Update completer by reconstructing all commands list.
  cmdList := make([]string, 0, len(i.Commands))
  for key := range i.Commands {
    cmdList = append(cmdList, key)
  }
  completer := func(line string) (c []string) {
    for _, n := range cmdList {
      if strings.HasPrefix(n, strings.ToLower(line)) {
        c = append(c, n)
      }
    }
    return
  }
  i.term.SetCompleter(completer)
  return nil
}

// setCommand sets global options into icli that can be accessed in
// GetOption. Without any params it prints the current set of global options.
func (i *ICli) setCommand(args []string) {
  if len(args) != 2 {
    fmt.Printf(i.GetGlobalOptions())
    return
  }
  err := i.SetGlobalOption(args[0], args[1])
  if err != nil {
    fmt.Printf("error: %v\n", err)
    return
  }
}

// unsetCommand clears a globally set option.
func (i *ICli) unsetCommand(args []string) {
  if len(args) != 2 {
    fmt.Printf("error: need one option to unset")
  }
  i.UnsetGlobalOption(args[1])
}

// updateOptionList updates the list of possible options based on the options
// from all commands. These are the valid options available to be set in the
// global options.
func (i *ICli) updateOptionList() error {
  // We do not know number of options so pick a random capacity.
  i.optionList = make(map[string]interface{})
  for _, cmd := range i.Commands {
    for _, opt := range cmd.Options {
      i.optionList[opt.Name] = struct{}{}
    }
  }
  for _, opt := range i.DefaultOptions {
    i.optionList[opt.Name] = struct{}{}
  }
  return nil
}
