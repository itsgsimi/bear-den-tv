# Qt6Gui requires CMake's FindOpenGL to succeed (WrapOpenGL). The pinned conda toolchain in
# toolchain/environment.yml ships the libGL.so.1 runtime but neither the unversioned libGL.so
# dev symlink nor GL headers, so FindOpenGL fails even though Qt itself links fine. This helper
# first tries the normal lookup and only then points FindOpenGL at the versioned runtime library
# with an empty include directory: the shell never includes GL headers directly. Adding
# libgl-devel + libglx-devel to the toolchain environment makes the fallback a no-op.
find_package(OpenGL QUIET)
if(NOT OpenGL_FOUND)
  set(OpenGL_GL_PREFERENCE LEGACY)
  find_library(OPENGL_gl_LIBRARY NAMES libGL.so.1 GL PATHS ENV CMAKE_PREFIX_PATH PATH_SUFFIXES lib)
  set(_bdtv_gl_shim "${CMAKE_BINARY_DIR}/gl-include-shim")
  file(MAKE_DIRECTORY "${_bdtv_gl_shim}")
  set(OPENGL_INCLUDE_DIR "${_bdtv_gl_shim}" CACHE PATH "Empty stand-in: the shell does not include GL headers" FORCE)
  find_package(OpenGL REQUIRED)
  message(STATUS "Bear Den TV: using OpenGL runtime fallback ${OPENGL_gl_LIBRARY} (toolchain lacks libgl-devel)")
endif()
