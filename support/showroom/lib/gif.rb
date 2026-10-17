# frozen_string_literal: true

require 'open3'
require_relative 'config'
require_relative 'progress'

module Showroom
  # Streams a lossless master through FFmpeg into Gifski, without frame files.
  class Gif
    include Progress

    def initialize(root)
      @config = Config.new(File.join(root, 'config/showroom.yml'))
      @settings = @config.gif
      validate_settings
    end

    def run(scene)
      directories(scene).each do |output|
        width = master_width(output)
        progress("Encoding GIF in #{output}") do
          statuses = Open3.pipeline(decoder, encoder(width), chdir: output)
          raise ArgumentError, "GIF encoding failed in #{output}" unless statuses.all?(&:success?)
        end
        puts "Saved #{File.join(output, 'preview.gif')}"
      end
    end

    private

    def directories(scene)
      outputs = @config.capture.fetch('themes').map { |theme| File.join(@config.output, theme, scene) }
      outputs.each do |output|
        next if File.file?(File.join(output, 'master.mkv'))

        raise ArgumentError, "missing master in #{output}; run showroom vid #{scene} first"
      end
      outputs
    end

    def validate_settings
      { 'fps' => 1..50, 'quality' => 1..100 }.each do |key, range|
        value = @settings.fetch(key)
        raise ArgumentError, "GIF #{key} must be an integer in #{range}" unless
          value.is_a?(Integer) && range.cover?(value)
      end
    end

    def decoder
      ['ffmpeg', '-nostdin', '-hide_banner', '-loglevel', 'error', '-i', 'master.mkv',
       '-an', '-vf', "fps=#{@settings.fetch('fps')}", '-pix_fmt', 'yuv444p', '-f', 'yuv4mpegpipe', '-']
    end

    def master_width(output)
      width, status = Open3.capture2('ffprobe', '-v', 'error', '-select_streams', 'v:0',
                                     '-show_entries', 'stream=width', '-of', 'csv=p=0',
                                     File.join(output, 'master.mkv'))
      raise ArgumentError, "cannot read master dimensions in #{output}" unless status.success?

      Integer(width.strip)
    end

    def encoder(width)
      ['gifski', '--quiet', '--fps', @settings.fetch('fps').to_s, '--quality', @settings.fetch('quality').to_s,
       '--width', width.to_s, '--repeat', '0', '--output', 'preview.gif', '-']
    end
  end
end
