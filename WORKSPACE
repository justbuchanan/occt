
new_local_repository(
    name="freetype",
    build_file_content = """
package(default_visibility=["//visibility:public"])
cc_library(
    name="freetype",
    hdrs=glob([
        "*.h",
        "config/*.h",
        "freetype/*.h",
        "freetype/config/*.h",
    ]),
    includes=["./", "freetype"],
)
    """,
    path="/usr/include/freetype2",
)


new_local_repository(
    name="tcl",
    build_file_content = """
package(default_visibility=["//visibility:public"])
cc_library(
    name="tcl",
    hdrs=glob([
        "*.h",
    ]),
    includes=["./"],
)
    """,
    path="/usr/include/tcl8.6",
)
