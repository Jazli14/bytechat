cmake_minimum_required(VERSION 3.20)

set(CMAKE_C_COMPILER clang)
set(CMAKE_CXX_COMPILER clang++)

project(bytechat-cpp VERSION 1.0)

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED True)

add_executable(bytechat main.cpp)

add_custom_target(run
    COMMAND bytechat
    DEPENDS bytechat
    WORKING_DIRECTORY ${bytechat-cpp}
)