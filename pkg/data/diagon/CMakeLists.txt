cmake_minimum_required(VERSION 3.15)
project(diagon_expression_evaluator VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Build type
if(NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release)
endif()

# Find dependencies
find_package(nlohmann_json 3.10.0 REQUIRED)

# Source files
set(DIAGON_SOURCES
    expression_evaluator.cpp
    document.cpp
    document_store.cpp
    search_integration.cpp
    shard_manager.cpp
    distributed_search.cpp
)

set(DIAGON_HEADERS
    expression_evaluator.h
    document.h
    document_store.h
    search_integration.h
    shard_manager.h
    distributed_search.h
)

# Expression evaluator library (shared for CGO)
add_library(diagon_expression SHARED ${DIAGON_SOURCES})

target_include_directories(diagon_expression
    PUBLIC
        $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}>
        $<INSTALL_INTERFACE:include>
)

target_link_libraries(diagon_expression
    PUBLIC
        nlohmann_json::nlohmann_json
)

# Compiler flags
target_compile_options(diagon_expression PRIVATE
    $<$<CONFIG:Release>:-O3 -march=native -ffast-math>
    $<$<CONFIG:Debug>:-O0 -g -Wall -Wextra>
)

# Position independent code (required for shared library)
set_target_properties(diagon_expression PROPERTIES
    POSITION_INDEPENDENT_CODE ON
    VERSION ${PROJECT_VERSION}
    SOVERSION 1
)

# Install rules
install(TARGETS diagon_expression
    EXPORT diagon_expression-targets
    LIBRARY DESTINATION lib
    ARCHIVE DESTINATION lib
    RUNTIME DESTINATION bin
)

install(FILES ${DIAGON_HEADERS}
    DESTINATION include/diagon
)

# Testing (optional)
option(BUILD_TESTS "Build unit tests" ON)
if(BUILD_TESTS)
    enable_testing()
    find_package(GTest QUIET)

    if(GTest_FOUND)
        add_executable(diagon_tests
            tests/document_test.cpp
            tests/expression_test.cpp
            tests/search_integration_test.cpp
        )

        target_link_libraries(diagon_tests
            PRIVATE
                diagon_expression
                GTest::gtest
                GTest::gtest_main
        )

        add_test(NAME diagon_tests COMMAND diagon_tests)
    else()
        message(STATUS "GTest not found - skipping tests")
    endif()
endif()

# Benchmarks (optional)
option(BUILD_BENCHMARKS "Build performance benchmarks" OFF)
if(BUILD_BENCHMARKS)
    find_package(benchmark QUIET)

    if(benchmark_FOUND)
        add_executable(diagon_benchmarks
            benchmarks/expression_benchmark.cpp
        )

        target_link_libraries(diagon_benchmarks
            PRIVATE
                diagon_expression
                benchmark::benchmark
                benchmark::benchmark_main
        )
    else()
        message(STATUS "Google Benchmark not found - skipping benchmarks")
    endif()
endif()

# Print configuration summary
message(STATUS "")
message(STATUS "Diagon Expression Evaluator Configuration:")
message(STATUS "  Build type: ${CMAKE_BUILD_TYPE}")
message(STATUS "  C++ standard: ${CMAKE_CXX_STANDARD}")
message(STATUS "  Build tests: ${BUILD_TESTS}")
message(STATUS "  Build benchmarks: ${BUILD_BENCHMARKS}")
message(STATUS "")
