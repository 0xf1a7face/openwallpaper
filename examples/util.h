#ifndef UTIL_H
#define UTIL_H

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include "openwallpaper.h"

typedef struct {
    uint8_t* data;
    size_t size;
} util_file;

static util_file util_load_file(const char* path) {
    FILE* file = fopen(path, "rb");
    if(file == NULL) {
        printf("error: failed to open %s\n", path);
        return (util_file){0};
    }
    if(fseek(file, 0, SEEK_END) != 0) {
        fclose(file);
        printf("error: failed to seek %s\n", path);
        return (util_file){0};
    }
    long end = ftell(file);
    if(end < 0) {
        fclose(file);
        printf("error: failed to tell %s\n", path);
        return (util_file){0};
    }
    if(fseek(file, 0, SEEK_SET) != 0) {
        fclose(file);
        printf("error: failed to rewind %s\n", path);
        return (util_file){0};
    }

    size_t size = (size_t)end;
    uint8_t* data = malloc(size == 0 ? 1 : size);
    if(data == NULL) {
        fclose(file);
        printf("error: failed to allocate %s\n", path);
        return (util_file){0};
    }
    if(fread(data, 1, size, file) != size) {
        free(data);
        fclose(file);
        printf("error: failed to read %s\n", path);
        return (util_file){0};
    }
    fclose(file);

    return (util_file){
        .data = data,
        .size = size,
    };
}

static ow_vertex_shader_id util_create_vertex_shader_from_file(const char* path) {
    util_file file = util_load_file(path);
    ow_vertex_shader_id shader = {0};
    if(file.data != NULL && file.size > 0) {
        shader = ow_create_vertex_shader(file.data, file.size);
    }
    free(file.data);
    return shader;
}

static ow_fragment_shader_id util_create_fragment_shader_from_file(const char* path) {
    util_file file = util_load_file(path);
    ow_fragment_shader_id shader = {0};
    if(file.data != NULL && file.size > 0) {
        shader = ow_create_fragment_shader(file.data, file.size);
    }
    free(file.data);
    return shader;
}

static ow_texture_id util_create_texture_from_file(const char* path, const ow_texture_info* info) {
    util_file file = util_load_file(path);
    ow_texture_id texture = {0};
    if(file.data != NULL && file.size > 0) {
        texture = ow_create_texture_from_image(file.data, file.size, info);
    }
    free(file.data);
    return texture;
}

#endif
