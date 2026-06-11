#version 450

layout(location = 0) in vec2 v_uv;
layout(location = 1) in vec4 v_color;
layout(location = 2) in flat int v_frame;
layout(location = 3) in flat float v_frame_blend;

layout(location = 0) out vec4 f_color;

layout(set = 2, binding = 0) uniform sampler2D u_texture;

layout(std140, set = 3, binding = 0) uniform uniforms_t {
    ivec2 spritesheet_size;
    vec2 screen_size;
};

vec4 convert_tex0_format(vec4 color) {
#if TEX0_FORMAT == 1
    return vec4(1.0, 1.0, 1.0, color.r);
#elif TEX0_FORMAT == 2
    return color.rrrg;
#else
    return color;
#endif
}

vec2 sprite_frame_uv(int frame, int cols, int rows) {
    int frame_x = frame % cols;
    int frame_y = frame / cols;
    float frame_width = 1.0 / float(cols);
    float frame_height = 1.0 / float(rows);
    return vec2(
        (float(frame_x) + v_uv.x) * frame_width,
        (float(frame_y) + v_uv.y) * frame_height
    );
}

void main() {
#if SPRITESHEET
    int cols = spritesheet_size.x;
    int rows = spritesheet_size.y;
    int frame_count = cols * rows;
    int current_frame = clamp(v_frame, 0, frame_count - 1);
    vec2 uv = sprite_frame_uv(current_frame, cols, rows);
#if FRAME_BLENDING
    int next_frame = min(frame_count - 1, current_frame + 1);
    vec4 tex_color = mix(
        convert_tex0_format(texture(u_texture, uv)),
        convert_tex0_format(texture(u_texture, sprite_frame_uv(next_frame, cols, rows))),
        v_frame_blend
    );
#else
    vec4 tex_color = convert_tex0_format(texture(u_texture, uv));
#endif
#else
    vec4 tex_color = convert_tex0_format(texture(u_texture, v_uv));
#endif
    f_color = tex_color * v_color;
}
