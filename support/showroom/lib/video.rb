# frozen_string_literal: true

require_relative 'config'
require_relative 'timeline'
require_relative 'progress'

module Showroom
  # Encodes existing PNGs into a lossless RGB master and a web playback preview.
  class Video
    include Progress

    def initialize(root)
      @root = root
      @config = Config.new(File.join(root, 'config/showroom.yml'))
      @settings = @config.video
      validate_settings
    end

    def run(scene)
      timeline = Timeline.new(File.join(@root, 'config/movies', "#{scene}.rb"), @settings.fetch('fps'))
      output_directories(scene, timeline).each do |output|
        encode_master(output, timeline)
        encode_preview(output)
        puts "Saved #{output}/{master.mkv,preview.mp4}"
      end
    end

    private

    # Check all themes before encoding, so a missing shot cannot leave a partial
    # batch of newly generated videos next to an older theme's videos.
    def output_directories(scene, timeline)
      outputs = @config.capture.fetch('themes').map { |theme| File.join(@config.output, theme, scene) }
      outputs.each { |output| validate_images(output, timeline, scene) }
      outputs
    end

    def validate_settings
      crf = @settings.fetch('preview_crf')
      raise ArgumentError, 'preview_crf must be an integer from 0 to 51' unless
        crf.is_a?(Integer) && (0..51).cover?(crf)

      presets = %w[ultrafast superfast veryfast faster fast medium slow slower veryslow placebo]
      raise ArgumentError, 'invalid preview_preset' unless presets.include?(@settings.fetch('preview_preset'))
    end

    def validate_images(output, timeline, scene)
      dimensions = timeline.shots.map do |shot|
        path = File.join(output, shot.fetch(:file))
        raise ArgumentError, "missing screenshot: #{path}; run showroom shot #{scene} first" unless File.file?(path)

        png_dimensions(path)
      end
      raise ArgumentError, "screenshots have different dimensions in #{output}; recapture the scene" unless
        dimensions.uniq.length == 1
    end

    def png_dimensions(path)
      header = File.binread(path, 24)
      raise ArgumentError, "invalid PNG: #{path}" unless
        header.bytesize == 24 && header.start_with?("\x89PNG\r\n\x1a\n".b) && header[12, 4] == 'IHDR'

      header[16, 8].unpack('NN')
    end

    def encode_master(output, timeline)
      inputs = timeline.shots.flat_map do |shot|
        ['-loop', '1', '-framerate', timeline.fps.to_s, '-i', shot.fetch(:file)]
      end
      options = ['-filter_complex_threads', '1', '-filter_complex', filters(timeline), '-map', '[video]',
                 '-frames:v', timeline.frames.to_s, '-c:v', 'ffv1', '-level', '3', '-pix_fmt', 'bgr0',
                 '-threads', '2', 'master.mkv']
      encode("Encoding master in #{output}", output, inputs + options)
    end

    def filters(timeline)
      trims = timeline.shots.each_with_index.map do |shot, i|
        "[#{i}:v]trim=end_frame=#{shot.fetch(:frames)},setpts=PTS-STARTPTS[s#{i}]"
      end
      labels = timeline.shots.each_index.map { |i| "[s#{i}]" }.join
      (trims + ["#{labels}concat=n=#{timeline.shots.length}:v=1:a=0,format=bgr0[video]"]).join(';')
    end

    def encode_preview(output)
      options = ['-i', 'master.mkv', '-an', '-vf', 'pad=ceil(iw/2)*2:ceil(ih/2)*2',
                 '-c:v', 'libx264', '-preset', @settings.fetch('preview_preset'),
                 '-crf', @settings.fetch('preview_crf').to_s, '-pix_fmt', 'yuv420p',
                 '-threads', '2', '-movflags', '+faststart', 'preview.mp4']
      encode("Encoding preview in #{output}", output, options)
    end

    def encode(label, output, options)
      command = ['ffmpeg', '-nostdin', '-hide_banner', '-loglevel', 'error', '-y'] + options
      progress(label) do
        raise ArgumentError, "video encoding failed in #{output}" unless system(*command, chdir: output)
      end
    end
  end
end
