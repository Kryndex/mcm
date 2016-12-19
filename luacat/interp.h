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

#ifndef MCM_LUACAT_INTERP_H_
#define MCM_LUACAT_INTERP_H_

#include <unistd.h>

#include "kj/common.h"
#include "kj/string.h"
#include "kj/vector.h"
#include "capnp/message.h"

extern "C" {
#include "lua.h"
}

#include "catalog.capnp.h"

namespace mcm {

namespace luacat {

class Lua {
  // The Lua interpreter.
  // Typical usage is one or more calls to exec followed by a call to finish.

public:
  Lua();
  KJ_DISALLOW_COPY(Lua);

  void exec(kj::StringPtr fname);
  // Run the Lua file at the given path.
  // Throws an exception if there is an error.

  Resource::Builder newResource();
  // (Mostly internal.) Add a new resource to the resulting catalog.

  void finish(capnp::MessageBuilder& message);
  // Build the catalog message.

  ~Lua();

private:
  lua_State* state;
  capnp::MallocMessageBuilder scratch;
  kj::Vector<capnp::Orphan<Resource>> resources;
};

}  // namespace luacat
}  // namespace mcm

#endif  // MCM_LUACAT_INTERP_H_