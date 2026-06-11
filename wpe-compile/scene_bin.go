package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

type sceneBinWriter struct {
	bytes.Buffer
	err error
}

type sceneObjectCommon struct {
	id            int
	parent        int
	visible       bool
	name          string
	attachment    string
	origin        Vector3
	scale         Vector3
	angles        Vector3
	size          Vector2
	perspective   bool
	parallaxDepth Vector2
}

func makeSceneBin(scene Scene) ([]byte, error) {
	writer := sceneBinWriter{}
	writer.scene(scene)
	if writer.err != nil {
		return nil, writer.err
	}
	return writer.Bytes(), nil
}

func (writer *sceneBinWriter) bytes(value []byte) {
	if writer.err != nil {
		return
	}
	_, writer.err = writer.Write(value)
}

func (writer *sceneBinWriter) bool(value bool) {
	if value {
		writer.u8(1)
	} else {
		writer.u8(0)
	}
}

func (writer *sceneBinWriter) u8(value uint8) {
	writer.bytes([]byte{value})
}

func (writer *sceneBinWriter) u32(value uint32) {
	data := [4]byte{}
	binary.LittleEndian.PutUint32(data[:], value)
	writer.bytes(data[:])
}

func (writer *sceneBinWriter) i32(value int) {
	writer.u32(uint32(int32(value)))
}

func (writer *sceneBinWriter) f32(value float32) {
	writer.u32(math.Float32bits(value))
}

func (writer *sceneBinWriter) string(value string) {
	if uint64(len(value)) > uint64(^uint32(0)) {
		writer.err = fmt.Errorf("string too large")
		return
	}
	writer.u32(uint32(len(value)))
	writer.bytes([]byte(value))
}

func (writer *sceneBinWriter) count(count int) {
	if count < 0 {
		writer.err = fmt.Errorf("negative count %d", count)
		return
	}
	writer.u32(uint32(count))
}

func (writer *sceneBinWriter) floats(values []float32) {
	writer.count(len(values))
	for _, value := range values {
		writer.f32(value)
	}
}

func (writer *sceneBinWriter) vec2(value Vector2) {
	writer.f32(value[0])
	writer.f32(value[1])
}

func (writer *sceneBinWriter) vec3(value Vector3) {
	writer.f32(value[0])
	writer.f32(value[1])
	writer.f32(value[2])
}

func (writer *sceneBinWriter) vec3Array(value [3]float32) {
	writer.f32(value[0])
	writer.f32(value[1])
	writer.f32(value[2])
}

func (writer *sceneBinWriter) scene(scene Scene) {
	writer.textures(scene.Textures)
	writer.shaders(scene.Shaders)
	writer.objects(scene.Objects, scene.Types)
	writer.general(scene.General)
	writer.i32(scene.PassthroughShader)
	writer.i32(scene.AudioSpectrumSize)
}

func (writer *sceneBinWriter) textures(textures []ImportTextureTask) {
	writer.count(len(textures))
	for _, texture := range textures {
		writer.i32(texture.ID)
		writer.string(texture.Name)
		writer.i32(texture.Width)
		writer.i32(texture.Height)
		writer.bool(texture.ClampUV)
		writer.bool(texture.Interpolation)
	}
}

func (writer *sceneBinWriter) shaders(shaders []CompileShaderTask) {
	writer.count(len(shaders))
	for _, shader := range shaders {
		writer.i32(shader.ID)
		writer.string(shader.Name)
		writer.uniforms(shader.VertexUniforms)
		writer.uniforms(shader.FragmentUniforms)
		writer.attributes(shader.Attributes)
		writer.samplers(shader.Samplers)
	}
}

func (writer *sceneBinWriter) uniforms(uniforms []UniformInfo) {
	writer.count(len(uniforms))
	for _, uniform := range uniforms {
		writer.string(uniform.Name)
		writer.string(uniform.ConstantName)
		writer.string(uniform.Type)
		writer.i32(uniform.ArraySize)
		writer.bool(uniform.DefaultSet && len(uniform.Default) > 0)
		if uniform.DefaultSet && len(uniform.Default) > 0 {
			writer.floats(uniform.Default)
		}
	}
}

func (writer *sceneBinWriter) attributes(attributes []AttributeInfo) {
	writer.count(len(attributes))
	for _, attribute := range attributes {
		writer.string(attribute.Name)
		writer.string(attribute.Type)
		writer.i32(attribute.ArraySize)
	}
}

func (writer *sceneBinWriter) samplers(samplers []SamplerInfo) {
	writer.count(len(samplers))
	for _, sampler := range samplers {
		writer.string(sampler.Name)
		writer.string(sampler.Default)
		writer.i32(sampler.TextureSlot)
	}
}

func (writer *sceneBinWriter) objects(objects []SceneObject, types []int) {
	writer.count(len(objects))
	for objectIndex, object := range objects {
		objectType := 2
		if objectIndex < len(types) {
			objectType = types[objectIndex]
		}
		writer.object(objectIndex, objectType, object)
	}
}

func (writer *sceneBinWriter) object(objectIndex int, objectType int, object SceneObject) {
	switch objectType {
	case 0:
		imageObject, ok := object.(*ImageObject)
		if !ok {
			writer.err = fmt.Errorf("object %d is marked image but has type %T", objectIndex, object)
			return
		}
		writer.objectCommon(sceneObjectCommon{
			id:            imageObject.ID,
			parent:        imageObject.Parent,
			visible:       imageObject.Visible,
			name:          imageObject.Name,
			attachment:    imageObject.Attachment,
			origin:        imageObject.Origin,
			scale:         imageObject.Scale,
			angles:        imageObject.Angles,
			size:          imageObject.Size,
			perspective:   imageObject.Perspective,
			parallaxDepth: imageObject.ParallaxDepth,
		}, objectType)
		writer.imageObject(imageObject)
	case 1:
		particleObject, ok := object.(*ParticleObject)
		if !ok {
			writer.err = fmt.Errorf("object %d is marked particle but has type %T", objectIndex, object)
			return
		}
		writer.objectCommon(sceneObjectCommon{
			id:            particleObject.ID,
			parent:        particleObject.Parent,
			visible:       particleObject.Visible,
			name:          particleObject.Name,
			attachment:    particleObject.Attachment,
			origin:        particleObject.Origin,
			scale:         particleObject.Scale,
			angles:        particleObject.Angles,
			size:          Vector2{2, 2},
			perspective:   particleObject.Perspective,
			parallaxDepth: particleObject.ParallaxDepth,
		}, objectType)
		writer.particleObject(particleObject)
	default:
		writer.objectCommon(emptyObjectCommon(object), 2)
	}
}

func emptyObjectCommon(object SceneObject) sceneObjectCommon {
	common := sceneObjectCommon{
		parent: -1,
		scale:  Vector3{1, 1, 1},
	}
	switch object := object.(type) {
	case *ImageObject:
		common.id = object.ID
		common.parent = object.Parent
		common.origin = object.Origin
		common.scale = object.Scale
		common.angles = object.Angles
	case *ParticleObject:
		common.id = object.ID
		common.parent = object.Parent
		common.origin = object.Origin
		common.scale = object.Scale
		common.angles = object.Angles
	case *EmptyObject:
		common.id = object.ID
		common.parent = object.Parent
		common.origin = object.Origin
		common.scale = object.Scale
		common.angles = object.Angles
	}
	return common
}

func (writer *sceneBinWriter) objectCommon(common sceneObjectCommon, objectType int) {
	writer.i32(common.id)
	writer.i32(common.parent)
	writer.bool(common.visible)
	writer.string(common.name)
	writer.string(common.attachment)
	writer.vec3(common.origin)
	writer.vec3(common.scale)
	writer.vec3(common.angles)
	writer.vec2(common.size)
	writer.bool(common.perspective)
	writer.vec2(common.parallaxDepth)
	writer.i32(objectType)
}

func (writer *sceneBinWriter) imageObject(object *ImageObject) {
	writer.vec3(object.Color)
	writer.i32(object.ColorBlendMode)
	writer.f32(object.Alpha)
	writer.f32(object.Brightness)
	writer.bool(object.Fullscreen)
	writer.bool(object.CompositionLayer)
	writer.bool(object.Config.Passthrough)
	writer.material(object.Material)
	writer.effects(object.Effects)
	if object.Puppet != nil && len(object.Effects) > 0 {
		writer.material(object.PuppetMaterial)
	} else {
		writer.material(Material{})
	}
	writer.puppet(object)
}

func (writer *sceneBinWriter) material(material Material) {
	writer.string(material.Blending)
	writer.i32(material.CompiledShader)
	writer.materialTextures(material.Textures, material.ImportedTextures)
}

func (writer *sceneBinWriter) materialTextures(names []string, textureIDs []int) {
	writer.count(len(textureIDs))
	for textureIndex, textureID := range textureIDs {
		name := ""
		if textureIndex < len(names) {
			name = names[textureIndex]
		}
		writer.string(name)
		writer.i32(textureID)
	}
}

func (writer *sceneBinWriter) effects(effects []ImageEffect) {
	writer.count(len(effects))
	for _, effect := range effects {
		writer.string(effect.Name)
		writer.bool(effect.Visible)
		writer.passes(effect.Passes)
		writer.fbos(effect.FBOs)
		writer.materials(effect.Materials)
	}
}

func (writer *sceneBinWriter) passes(passes []MaterialPass) {
	writer.count(len(passes))
	for _, pass := range passes {
		writer.materialTextures(pass.Textures, pass.ImportedTextures)
		writer.constants(pass.Constants)
		writer.string(pass.Target)
		writer.binds(pass.Bind)
	}
}

func (writer *sceneBinWriter) constants(constants map[string][]float32) {
	names := make([]string, 0, len(constants))
	for name := range constants {
		names = append(names, name)
	}
	sort.Strings(names)

	writer.count(len(names))
	for _, name := range names {
		writer.string(name)
		writer.floats(constants[name])
	}
}

func (writer *sceneBinWriter) binds(binds []MaterialPassBindItem) {
	writer.count(len(binds))
	for _, bind := range binds {
		writer.string(bind.Name)
		writer.i32(bind.Index)
	}
}

func (writer *sceneBinWriter) fbos(fbos []EffectFBO) {
	writer.count(len(fbos))
	for _, fbo := range fbos {
		writer.string(fbo.Name)
		writer.i32(fbo.Scale)
	}
}

func (writer *sceneBinWriter) materials(materials []Material) {
	writer.count(len(materials))
	for _, material := range materials {
		writer.material(material)
	}
}

func (writer *sceneBinWriter) puppet(object *ImageObject) {
	if object.Puppet == nil {
		writer.bool(false)
		return
	}
	writer.bool(true)
	writer.string(object.Puppet.Path)
	writer.i32(object.Puppet.BoneCount)
	writer.puppetLayers(object.PuppetLayers)
}

func (writer *sceneBinWriter) puppetLayers(layers []PuppetAnimationLayer) {
	writer.count(len(layers))
	for _, layer := range layers {
		writer.i32(layer.ID)
		writer.f32(layer.Blend)
		writer.f32(layer.Rate)
		writer.bool(layer.Visible)
		writer.bool(layer.Additive)
		writer.bool(layer.BlendIn)
		writer.bool(layer.BlendOut)
		writer.f32(layer.BlendTime)
	}
}

func (writer *sceneBinWriter) particleObject(object *ParticleObject) {
	particle := object.ParticleData
	writer.material(particle.Material)
	writer.particleEmitters(particle.Emitters)
	writer.particleInitializer(particle.Initializer)
	writer.particleOperator(particle.Operator)
	writer.u32(particle.MaxCount)
	writer.f32(particle.StartTime)
	writer.f32(particle.SequenceMultiplier)
	writer.bool(particle.RandomFrame)
	writer.bool(particleFrameBlendingEnabled(object))
	writer.i32(object.SpritesheetCols)
	writer.i32(object.SpritesheetRows)
	writer.i32(object.SpritesheetFrames)
	writer.f32(0)
	writer.f32(object.TextureRatio)
}

func (writer *sceneBinWriter) particleEmitters(emitters []ParticleEmitter) {
	writer.count(len(emitters))
	for _, emitter := range emitters {
		writer.vec3(emitter.Directions)
		writer.vec3(emitter.DistanceMax)
		writer.vec3(emitter.DistanceMin)
		writer.vec3(emitter.Origin)
		for _, sign := range emitter.Sign {
			writer.i32(int(sign))
		}
		writer.f32(emitter.SpeedMin)
		writer.f32(emitter.SpeedMax)
		writer.f32(emitter.Rate)
	}
}

func (writer *sceneBinWriter) particleInitializer(init ParticleInitializer) {
	writer.f32(init.MinLifetime)
	writer.f32(init.MaxLifetime)
	writer.f32(init.MinSize)
	writer.f32(init.MaxSize)
	writer.vec3Array(init.MinVelocity)
	writer.vec3Array(init.MaxVelocity)
	writer.vec3Array(init.MinRotation)
	writer.vec3Array(init.MaxRotation)
	writer.vec3Array(init.MinAngularVelocity)
	writer.vec3Array(init.MaxAngularVelocity)
	writer.vec3Array(init.MinColor)
	writer.vec3Array(init.MaxColor)
	writer.f32(init.MinAlpha)
	writer.f32(init.MaxAlpha)
	writer.bool(init.TurbulentVelocity)
	writer.f32(init.TurbulentScale)
	writer.f32(init.TurbulentTimeScale)
	writer.f32(init.TurbulentOffset)
	writer.f32(init.TurbulentSpeedMin)
	writer.f32(init.TurbulentSpeedMax)
	writer.f32(init.TurbulentPhaseMin)
	writer.f32(init.TurbulentPhaseMax)
	writer.vec3(init.TurbulentForward)
	writer.vec3(init.TurbulentRight)
	writer.turbulentAudio(init.TurbulentAudio)
}

func (writer *sceneBinWriter) turbulentAudio(audio TurbulentAudioResponse) {
	writer.i32(int(audio.Mode))
	writer.f32(audio.Exponent)
	writer.vec2(audio.Bounds)
	writer.i32(int(audio.FrequencyStart))
	writer.i32(int(audio.FrequencyEnd))
}

func (writer *sceneBinWriter) particleOperator(operator ParticleOperator) {
	writer.bool(operator.Movement.Enabled)
	writer.vec3(operator.Movement.Gravity)
	writer.f32(operator.Movement.Drag)
	writer.f32(operator.Movement.Speed)

	writer.bool(operator.AngularMovement.Enabled)
	writer.f32(operator.AngularMovement.Drag)
	writer.vec3(operator.AngularMovement.Force)

	writer.bool(operator.SizeChange.Enabled)
	writer.f32(operator.SizeChange.StartTime)
	writer.f32(operator.SizeChange.EndTime)
	writer.f32(operator.SizeChange.StartValue)
	writer.f32(operator.SizeChange.EndValue)

	writer.bool(operator.ColorChange.Enabled)
	writer.f32(operator.ColorChange.StartTime)
	writer.f32(operator.ColorChange.EndTime)
	writer.vec3(operator.ColorChange.StartValue)
	writer.vec3(operator.ColorChange.EndValue)

	writer.bool(operator.AlphaFade.Enabled)
	writer.f32(operator.AlphaFade.FadeInTime)
	writer.f32(operator.AlphaFade.FadeOutTime)

	writer.bool(operator.OscillatePosition.Enabled)
	writer.vec3(operator.OscillatePosition.Mask)
	writer.f32(operator.OscillatePosition.FrequencyMin)
	writer.f32(operator.OscillatePosition.FrequencyMax)
	writer.f32(operator.OscillatePosition.PhaseMin)
	writer.f32(operator.OscillatePosition.PhaseMax)
	writer.f32(operator.OscillatePosition.ScaleMin)
	writer.f32(operator.OscillatePosition.ScaleMax)

	writer.oscillateScalar(operator.OscillateAlpha)
	writer.oscillateScalar(operator.OscillateSize)
}

func (writer *sceneBinWriter) oscillateScalar(operator OscillateScalarOperator) {
	writer.bool(operator.Enabled)
	writer.f32(operator.FrequencyMin)
	writer.f32(operator.FrequencyMax)
	writer.f32(operator.PhaseMin)
	writer.f32(operator.PhaseMax)
	writer.f32(operator.ScaleMin)
	writer.f32(operator.ScaleMax)
}

func (writer *sceneBinWriter) general(general SceneGeneral) {
	writer.bool(general.Parallax)
	writer.f32(general.ParallaxAmount)
	writer.f32(general.ParallaxDelay)
	writer.f32(general.ParallaxMouseInfluence)
	writer.bool(general.Shake)
	writer.f32(general.ShakeAmplitude)
	writer.f32(general.ShakeRoughness)
	writer.f32(general.ShakeSpeed)
	writer.bool(general.ClearEnabled)
	writer.vec3(general.ClearColor)
	writer.i32(general.Ortho.Width)
	writer.i32(general.Ortho.Height)
	writer.f32(general.Zoom)
	writer.f32(general.FOV)
	writer.f32(general.NearZ)
	writer.f32(general.FarZ)
}
