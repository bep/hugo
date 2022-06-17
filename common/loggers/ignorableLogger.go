// Copyright 2020 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package loggers

import (
	"fmt"
	"strings"
)

// IgnorableLogger is a logger that ignores certain log statements.
type IgnorableLogger interface {
	Logger
<<<<<<< HEAD
	Errorsf(statementID, format string, v ...any)
=======
	Errorsf(statementID, format string, v ...interface{})
	Warnsf(statementID, format string, v ...interface{})
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	Apply(logger Logger) IgnorableLogger
}

type ignorableLogger struct {
	Logger
	statementsError   map[string]bool
	statementsWarning map[string]bool
}

// NewIgnorableLogger wraps the given logger and ignores the log statement IDs given.
func NewIgnorableLogger(logger Logger, statementsError, statementsWarning []string) IgnorableLogger {
	statementsSetError := make(map[string]bool)
	for _, s := range statementsError {
		statementsSetError[strings.ToLower(s)] = true
	}
	statementsSetWarning := make(map[string]bool)
	for _, s := range statementsWarning {
		statementsSetWarning[strings.ToLower(s)] = true
	}
	return ignorableLogger{
		Logger:            logger,
		statementsError:   statementsSetError,
		statementsWarning: statementsSetWarning,
	}
}

// Errorsf logs statementID as an ERROR if not configured as ignoreable.
<<<<<<< HEAD
func (l ignorableLogger) Errorsf(statementID, format string, v ...any) {
	if l.statements[statementID] {
=======
func (l ignorableLogger) Errorsf(statementID, format string, v ...interface{}) {
	if l.statementsError[statementID] {
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		// Ignore.
		return
	}
	ignoreMsg := fmt.Sprintf(`
If you feel that this should not be logged as an ERROR, you can ignore it by adding this to your site config:
ignoreErrors = [%q]`, statementID)

	format += ignoreMsg

	l.Errorf(format, v...)
}

// Warnsf logs statementID as an WARNING if not configured as ignoreable.
func (l ignorableLogger) Warnsf(statementID, format string, v ...interface{}) {
	if l.statementsWarning[statementID] {
		// Ignore.
		return
	}
	ignoreMsg := fmt.Sprintf(`
To turn off this WARNING, you can ignore it by adding this to your site config:
ignoreWarnings = [%q]`, statementID)

	format += ignoreMsg

	l.Warnf(format, v...)
}

func (l ignorableLogger) Apply(logger Logger) IgnorableLogger {
	return ignorableLogger{
		Logger:          logger,
		statementsError: l.statementsError,
	}
}
