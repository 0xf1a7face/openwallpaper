#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "defs.h"

typedef struct {
    uint8_t* data;
    size_t size;
    size_t offset;
    bool failed;
} wpe_scene_bin_reader;

wpe_scene scene = {0};

static bool read_bytes(wpe_scene_bin_reader* reader, void* out, size_t size) {
    if(reader->failed || size > reader->size - reader->offset) {
        reader->failed = true;
        return false;
    }
    if(out != NULL && size > 0) {
        memcpy(out, reader->data + reader->offset, size);
    }
    reader->offset += size;
    return true;
}

static uint8_t read_u8(wpe_scene_bin_reader* reader) {
    uint8_t value = 0;
    (void)read_bytes(reader, &value, sizeof(value));
    return value;
}

static uint32_t read_u32(wpe_scene_bin_reader* reader) {
    uint8_t data[4] = {0};
    (void)read_bytes(reader, data, sizeof(data));
    return (uint32_t)data[0] | ((uint32_t)data[1] << 8) | ((uint32_t)data[2] << 16) | ((uint32_t)data[3] << 24);
}

static int read_i32(wpe_scene_bin_reader* reader) {
    return (int)(int32_t)read_u32(reader);
}

static float read_f32(wpe_scene_bin_reader* reader) {
    uint32_t bits = read_u32(reader);
    float value = 0.0f;
    memcpy(&value, &bits, sizeof(value));
    return value;
}

static bool read_bool(wpe_scene_bin_reader* reader) {
    return read_u8(reader) != 0;
}

static int read_count(wpe_scene_bin_reader* reader) {
    uint32_t count = read_u32(reader);
    if(count > INT_MAX) {
        reader->failed = true;
        return 0;
    }
    return (int)count;
}

static void* read_array(wpe_scene_bin_reader* reader, int count, size_t size) {
    if(count <= 0) {
        return NULL;
    }
    void* data = calloc((size_t)count, size);
    if(data == NULL) {
        reader->failed = true;
    }
    return data;
}

static char* read_string(wpe_scene_bin_reader* reader) {
    uint32_t len = read_u32(reader);
    if(reader->failed || len > reader->size - reader->offset) {
        reader->failed = true;
        return NULL;
    }

    char* value = malloc((size_t)len + 1);
    if(value == NULL) {
        reader->failed = true;
        return NULL;
    }
    if(len > 0) {
        memcpy(value, reader->data + reader->offset, len);
    }
    value[len] = '\0';
    reader->offset += len;
    return value;
}

static void read_vec2(wpe_scene_bin_reader* reader, wpe_vec2* value) {
    value->x = read_f32(reader);
    value->y = read_f32(reader);
}

static void read_vec3(wpe_scene_bin_reader* reader, wpe_vec3* value) {
    value->x = read_f32(reader);
    value->y = read_f32(reader);
    value->z = read_f32(reader);
}

static void read_float_array(wpe_scene_bin_reader* reader, float* values, int count) {
    for(int i = 0; i < count; i++) {
        values[i] = read_f32(reader);
    }
}

static float* read_float_values(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    float* values = read_array(reader, count, sizeof(float));
    for(int i = 0; i < count; i++) {
        values[i] = read_f32(reader);
    }
    *count_out = count;
    return values;
}

static wpe_material_texture* read_material_textures(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_material_texture* textures = read_array(reader, count, sizeof(wpe_material_texture));
    for(int i = 0; i < count; i++) {
        textures[i].name = read_string(reader);
        textures[i].texture_id = read_i32(reader);
    }
    *count_out = count;
    return textures;
}

static wpe_uniform_info* read_uniforms(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_uniform_info* uniforms = read_array(reader, count, sizeof(wpe_uniform_info));
    for(int i = 0; i < count; i++) {
        uniforms[i].name = read_string(reader);
        uniforms[i].constant_name = read_string(reader);
        uniforms[i].type = read_string(reader);
        uniforms[i].array_size = read_i32(reader);
        uniforms[i].default_set = read_bool(reader);
        if(uniforms[i].default_set) {
            uniforms[i].default_value = read_float_values(reader, &uniforms[i].default_len);
        }
    }
    *count_out = count;
    return uniforms;
}

static wpe_attribute_info* read_attributes(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_attribute_info* attributes = read_array(reader, count, sizeof(wpe_attribute_info));
    for(int i = 0; i < count; i++) {
        attributes[i].name = read_string(reader);
        attributes[i].type = read_string(reader);
        attributes[i].array_size = read_i32(reader);
    }
    *count_out = count;
    return attributes;
}

static wpe_sampler_info* read_samplers(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_sampler_info* samplers = read_array(reader, count, sizeof(wpe_sampler_info));
    for(int i = 0; i < count; i++) {
        samplers[i].name = read_string(reader);
        samplers[i].default_texture = read_string(reader);
        samplers[i].texture_slot = read_i32(reader);
    }
    *count_out = count;
    return samplers;
}

static void read_material(wpe_scene_bin_reader* reader, wpe_material* material) {
    material->blending = read_string(reader);
    material->shader_id = read_i32(reader);
    material->textures = read_material_textures(reader, &material->num_textures);
}

static wpe_uniform_constant* read_constants(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_uniform_constant* constants = read_array(reader, count, sizeof(wpe_uniform_constant));
    for(int i = 0; i < count; i++) {
        constants[i].name = read_string(reader);
        constants[i].values = read_float_values(reader, &constants[i].len);
    }
    *count_out = count;
    return constants;
}

static wpe_material_pass_bind* read_binds(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_material_pass_bind* binds = read_array(reader, count, sizeof(wpe_material_pass_bind));
    for(int i = 0; i < count; i++) {
        binds[i].name = read_string(reader);
        binds[i].index = read_i32(reader);
    }
    *count_out = count;
    return binds;
}

static wpe_material_pass* read_passes(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_material_pass* passes = read_array(reader, count, sizeof(wpe_material_pass));
    for(int i = 0; i < count; i++) {
        passes[i].textures = read_material_textures(reader, &passes[i].num_textures);
        passes[i].constants = read_constants(reader, &passes[i].num_constants);
        passes[i].target = read_string(reader);
        passes[i].binds = read_binds(reader, &passes[i].num_binds);
    }
    *count_out = count;
    return passes;
}

static wpe_effect_fbo* read_fbos(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_effect_fbo* fbos = read_array(reader, count, sizeof(wpe_effect_fbo));
    for(int i = 0; i < count; i++) {
        fbos[i].name = read_string(reader);
        fbos[i].scale = read_i32(reader);
    }
    *count_out = count;
    return fbos;
}

static wpe_material* read_materials(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_material* materials = read_array(reader, count, sizeof(wpe_material));
    for(int i = 0; i < count; i++) {
        read_material(reader, &materials[i]);
    }
    *count_out = count;
    return materials;
}

static wpe_image_effect* read_effects(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_image_effect* effects = read_array(reader, count, sizeof(wpe_image_effect));
    for(int i = 0; i < count; i++) {
        effects[i].name = read_string(reader);
        effects[i].visible = read_bool(reader);
        effects[i].passes = read_passes(reader, &effects[i].num_passes);
        effects[i].fbos = read_fbos(reader, &effects[i].num_fbos);
        effects[i].materials = read_materials(reader, &effects[i].num_materials);
    }
    *count_out = count;
    return effects;
}

static wpe_puppet_animation_layer* read_puppet_layers(wpe_scene_bin_reader* reader, int* count_out) {
    int count = read_count(reader);
    wpe_puppet_animation_layer* layers = read_array(reader, count, sizeof(wpe_puppet_animation_layer));
    for(int i = 0; i < count; i++) {
        layers[i].id = read_i32(reader);
        layers[i].blend = read_f32(reader);
        layers[i].rate = read_f32(reader);
        layers[i].visible = read_bool(reader);
        layers[i].additive = read_bool(reader);
        layers[i].blend_in = read_bool(reader);
        layers[i].blend_out = read_bool(reader);
        layers[i].blend_time = read_f32(reader);
    }
    *count_out = count;
    return layers;
}

static wpe_puppet_model* read_puppet(wpe_scene_bin_reader* reader) {
    bool has_puppet = read_bool(reader);
    if(!has_puppet) {
        return NULL;
    }

    wpe_puppet_model* puppet = calloc(1, sizeof(wpe_puppet_model));
    if(puppet == NULL) {
        reader->failed = true;
        return NULL;
    }
    puppet->path = read_string(reader);
    puppet->num_bones = read_i32(reader);
    puppet->layers = read_puppet_layers(reader, &puppet->num_layers);
    return puppet;
}

static void read_particle_emitters(wpe_scene_bin_reader* reader, wpe_particle_object* particle) {
    particle->num_emitters = read_count(reader);
    particle->emitters = read_array(reader, particle->num_emitters, sizeof(wpe_particle_emitter));
    for(int i = 0; i < particle->num_emitters; i++) {
        read_float_array(reader, particle->emitters[i].directions, 3);
        read_float_array(reader, particle->emitters[i].distance_max, 3);
        read_float_array(reader, particle->emitters[i].distance_min, 3);
        read_float_array(reader, particle->emitters[i].origin, 3);
        for(int axis = 0; axis < 3; axis++) {
            particle->emitters[i].sign[axis] = read_i32(reader);
        }
        particle->emitters[i].speed_min = read_f32(reader);
        particle->emitters[i].speed_max = read_f32(reader);
        particle->emitters[i].rate = read_f32(reader);
    }
}

static void read_particle_initializer(wpe_scene_bin_reader* reader, wpe_particle_initializer* init) {
    init->min_lifetime = read_f32(reader);
    init->max_lifetime = read_f32(reader);
    init->min_size = read_f32(reader);
    init->max_size = read_f32(reader);
    read_float_array(reader, init->min_velocity, 3);
    read_float_array(reader, init->max_velocity, 3);
    read_float_array(reader, init->min_rotation, 3);
    read_float_array(reader, init->max_rotation, 3);
    read_float_array(reader, init->min_angular_velocity, 3);
    read_float_array(reader, init->max_angular_velocity, 3);
    read_float_array(reader, init->min_color, 3);
    read_float_array(reader, init->max_color, 3);
    init->min_alpha = read_f32(reader);
    init->max_alpha = read_f32(reader);
    init->turbulent_velocity = read_bool(reader);
    init->turbulent_scale = read_f32(reader);
    init->turbulent_time_scale = read_f32(reader);
    init->turbulent_offset = read_f32(reader);
    init->turbulent_speed_min = read_f32(reader);
    init->turbulent_speed_max = read_f32(reader);
    init->turbulent_phase_min = read_f32(reader);
    init->turbulent_phase_max = read_f32(reader);
    read_float_array(reader, init->turbulent_forward, 3);
    read_float_array(reader, init->turbulent_right, 3);
    init->turbulent_audio.mode = read_i32(reader);
    init->turbulent_audio.exponent = read_f32(reader);
    read_float_array(reader, init->turbulent_audio.bounds, 2);
    init->turbulent_audio.frequency_start = read_i32(reader);
    init->turbulent_audio.frequency_end = read_i32(reader);
}

static void read_oscillate_scalar(wpe_scene_bin_reader* reader, wpe_particle_oscillate_scalar_operator* op) {
    op->enabled = read_bool(reader);
    op->frequency_min = read_f32(reader);
    op->frequency_max = read_f32(reader);
    op->phase_min = read_f32(reader);
    op->phase_max = read_f32(reader);
    op->scale_min = read_f32(reader);
    op->scale_max = read_f32(reader);
}

static void read_particle_operator(wpe_scene_bin_reader* reader, wpe_particle_operator* op) {
    op->movement.enabled = read_bool(reader);
    read_float_array(reader, op->movement.gravity, 3);
    op->movement.drag = read_f32(reader);
    op->movement.speed = read_f32(reader);

    op->angular_movement.enabled = read_bool(reader);
    op->angular_movement.drag = read_f32(reader);
    read_float_array(reader, op->angular_movement.force, 3);

    op->size_change.enabled = read_bool(reader);
    op->size_change.start_time = read_f32(reader);
    op->size_change.end_time = read_f32(reader);
    op->size_change.start_value = read_f32(reader);
    op->size_change.end_value = read_f32(reader);

    op->color_change.enabled = read_bool(reader);
    op->color_change.start_time = read_f32(reader);
    op->color_change.end_time = read_f32(reader);
    read_float_array(reader, op->color_change.start_value, 3);
    read_float_array(reader, op->color_change.end_value, 3);

    op->alpha_fade.enabled = read_bool(reader);
    op->alpha_fade.fade_in_time = read_f32(reader);
    op->alpha_fade.fade_out_time = read_f32(reader);

    op->oscillate_position.enabled = read_bool(reader);
    read_float_array(reader, op->oscillate_position.mask, 3);
    op->oscillate_position.frequency_min = read_f32(reader);
    op->oscillate_position.frequency_max = read_f32(reader);
    op->oscillate_position.phase_min = read_f32(reader);
    op->oscillate_position.phase_max = read_f32(reader);
    op->oscillate_position.scale_min = read_f32(reader);
    op->oscillate_position.scale_max = read_f32(reader);

    read_oscillate_scalar(reader, &op->oscillate_alpha);
    read_oscillate_scalar(reader, &op->oscillate_size);
}

static void read_particle_object(wpe_scene_bin_reader* reader, wpe_particle_object* particle) {
    read_material(reader, &particle->material);
    read_particle_emitters(reader, particle);
    read_particle_initializer(reader, &particle->init);
    read_particle_operator(reader, &particle->operator);

    uint32_t max_count = read_u32(reader);
    if(max_count > INT_MAX) {
        reader->failed = true;
    }
    particle->max_count = (int)max_count;
    particle->start_time = read_f32(reader);
    particle->sequence_multiplier = read_f32(reader);
    particle->random_frame = read_bool(reader);
    particle->frame_blending = read_bool(reader);
    particle->spritesheet_cols = read_i32(reader);
    particle->spritesheet_rows = read_i32(reader);
    particle->spritesheet_frames = read_i32(reader);
    particle->spritesheet_duration = read_f32(reader);
    particle->texture_ratio = read_f32(reader);
}

static void read_object_common(wpe_scene_bin_reader* reader, wpe_object* object) {
    object->id = read_i32(reader);
    object->parent = read_i32(reader);
    object->visible = read_bool(reader);
    object->name = read_string(reader);
    object->attachment = read_string(reader);
    read_vec3(reader, &object->origin);
    read_vec3(reader, &object->scale);
    read_vec3(reader, &object->angles);
    read_vec2(reader, &object->size);
    object->perspective = read_bool(reader);
    read_vec2(reader, &object->parallax_depth);
    object->type = (wpe_object_type)read_i32(reader);
}

static void read_image_object(wpe_scene_bin_reader* reader, wpe_image_object* image) {
    read_vec3(reader, &image->color);
    image->color_blend_mode = read_i32(reader);
    image->alpha = read_f32(reader);
    image->brightness = read_f32(reader);
    image->fullscreen = read_bool(reader);
    image->composition_layer = read_bool(reader);
    image->passthrough = read_bool(reader);
    read_material(reader, &image->material);
    image->effects = read_effects(reader, &image->num_effects);
    read_material(reader, &image->puppet_material);
    image->puppet = read_puppet(reader);
}

static void read_objects(wpe_scene_bin_reader* reader, wpe_scene* scene_out) {
    int count = read_count(reader);
    scene_out->num_objects = (size_t)count;
    scene_out->objects = read_array(reader, count, sizeof(wpe_object));
    for(int i = 0; i < count; i++) {
        read_object_common(reader, &scene_out->objects[i]);
        if(scene_out->objects[i].type == OBJECTTYPE_IMAGE) {
            read_image_object(reader, &scene_out->objects[i].image);
        } else if(scene_out->objects[i].type == OBJECTTYPE_PARTICLE) {
            read_particle_object(reader, &scene_out->objects[i].particle);
        } else if(scene_out->objects[i].type != OBJECTTYPE_EMPTY) {
            reader->failed = true;
            return;
        }
    }
}

static void read_textures(wpe_scene_bin_reader* reader, wpe_scene* scene_out) {
    int count = read_count(reader);
    scene_out->num_textures = (size_t)count;
    scene_out->textures = read_array(reader, count, sizeof(wpe_texture));
    for(int i = 0; i < count; i++) {
        scene_out->textures[i].id = read_i32(reader);
        scene_out->textures[i].name = read_string(reader);
        scene_out->textures[i].width = read_i32(reader);
        scene_out->textures[i].height = read_i32(reader);
        scene_out->textures[i].clamp_uv = read_bool(reader);
        scene_out->textures[i].interpolation = read_bool(reader);
    }
}

static void read_shaders(wpe_scene_bin_reader* reader, wpe_scene* scene_out) {
    int count = read_count(reader);
    scene_out->num_shaders = (size_t)count;
    scene_out->shaders = read_array(reader, count, sizeof(wpe_shader));
    for(int i = 0; i < count; i++) {
        scene_out->shaders[i].id = read_i32(reader);
        scene_out->shaders[i].name = read_string(reader);
        scene_out->shaders[i].vertex_uniforms = read_uniforms(reader, &scene_out->shaders[i].num_vertex_uniforms);
        scene_out->shaders[i].fragment_uniforms = read_uniforms(reader, &scene_out->shaders[i].num_fragment_uniforms);
        scene_out->shaders[i].attributes = read_attributes(reader, &scene_out->shaders[i].num_attributes);
        scene_out->shaders[i].samplers = read_samplers(reader, &scene_out->shaders[i].num_samplers);
    }
}

static void read_general(wpe_scene_bin_reader* reader, wpe_scene_general* general) {
    general->parallax = read_bool(reader);
    general->parallax_amount = read_f32(reader);
    general->parallax_delay = read_f32(reader);
    general->parallax_mouse_influence = read_f32(reader);
    general->shake = read_bool(reader);
    general->shake_amplitude = read_f32(reader);
    general->shake_roughness = read_f32(reader);
    general->shake_speed = read_f32(reader);
    general->clear_enabled = read_bool(reader);
    read_vec3(reader, &general->clear_color);
    general->ortho.w = (float)read_i32(reader);
    general->ortho.h = (float)read_i32(reader);
    general->zoom = read_f32(reader);
    general->fov = read_f32(reader);
    general->near_z = read_f32(reader);
    general->far_z = read_f32(reader);
}

static void read_scene_bin(wpe_scene_bin_reader* reader, wpe_scene* scene_out) {
    read_textures(reader, scene_out);
    read_shaders(reader, scene_out);
    read_objects(reader, scene_out);
    read_general(reader, &scene_out->general);
    scene_out->passthrough_shader_id = read_i32(reader);
    scene_out->audio_spectrum_size = read_i32(reader);
    if(reader->offset != reader->size) {
        reader->failed = true;
    }
}

bool wpe_load_scene() {
    static bool loaded = false;
    if(loaded) {
        return true;
    }

    size_t size = ow_get_file_size("scene.bin");
    if(size == 0) {
        printf("error: scene.bin is missing or empty\n");
        return false;
    }

    uint8_t* data = malloc(size);
    if(data == NULL) {
        printf("error: cannot allocate scene data\n");
        return false;
    }
    ow_read_file("scene.bin", data);

    wpe_scene_bin_reader reader = {
        .data = data,
        .size = size,
    };
    read_scene_bin(&reader, &scene);
    free(data);

    if(reader.failed) {
        memset(&scene, 0, sizeof(scene));
        printf("error: cannot parse scene.bin\n");
        return false;
    }

    loaded = true;
    return true;
}
