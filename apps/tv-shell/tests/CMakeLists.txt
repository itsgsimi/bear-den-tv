# Offscreen shell tests: they load the shipped `BearDen` module (not copies)
# and drive it through Nav exactly as coordinator input does.
qt_add_executable(tst_shell tst_shell.cpp)
target_link_libraries(tst_shell PRIVATE beardenshell beardenshellplugin Qt6::Test)
target_compile_definitions(tst_shell PRIVATE BDTV_FIXTURE_DIR="${CMAKE_CURRENT_SOURCE_DIR}/fixtures"
                                             BDTV_CONTRACT_FIXTURE_DIR="${CMAKE_CURRENT_SOURCE_DIR}/../../../contracts/fixtures")
add_test(NAME tst_shell COMMAND tst_shell)
set_tests_properties(tst_shell PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen")
