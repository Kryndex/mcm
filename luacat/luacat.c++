// Copyright 2016 The Minimal Configuration Manager Authors
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

#include <unistd.h>
#include <stdlib.h>
#include "kj/io.h"
#include "kj/main.h"
#include "kj/miniposix.h"

#include "luacat/main.h"

int main(int argc, char* argv[]) {
  kj::TopLevelProcessContext context(argv[0]);
  kj::FdOutputStream out(STDOUT_FILENO);
  kj::FdOutputStream err(STDERR_FILENO);
  mcm::luacat::Main mainObject(context, out, err);
  const char* path = getenv("MCM_LUACAT_PATH");
  if (path != nullptr) {
    mainObject.setFallbackIncludePath(path);
  }
  return kj::runMainAndExit(context, mainObject.getMain(), argc, argv);
}
