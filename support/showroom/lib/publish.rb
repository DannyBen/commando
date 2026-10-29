# frozen_string_literal: true

require 'fileutils'
require_relative 'config'

module Showroom
  # Publishes approved scene frames and videos to the static site.
  class Publish
    POSTERS = { 'panels' => '04-filtered-media', 'themes' => '03-seed-44',
                'commands' => '03-search-rename' }.freeze

    def initialize(root)
      @root = File.expand_path(root)
      @config = Config.new(File.join(@root, 'config/showroom.yml'))
      @assets = File.expand_path('../../site/assets', @root)
    end

    def run
      sources = source_files
      sources.each_value do |source|
        raise ArgumentError, "missing showcase asset: #{source}" unless File.file?(source)
      end

      FileUtils.mkdir_p(@assets)
      sources.each do |target, source|
        if target.end_with?('.jpg')
          convert_poster(source, File.join(@assets, target))
        else
          FileUtils.cp(source, File.join(@assets, target))
        end
      end
      puts "Published #{sources.length} showcase assets to #{@assets}"
    end

    private

    def source_files
      @config.capture.fetch('themes').each_with_object({}) do |theme, files|
        files["overview-#{theme}.png"] = source(theme, 'overview', 'overview.png')
        POSTERS.each do |scene, frame|
          files["#{scene}-#{theme}.mp4"] = source(theme, scene, 'preview.mp4')
          files["#{scene}-#{theme}.jpg"] = source(theme, scene, "#{frame}.png")
        end
      end
    end

    def source(theme, scene, file)
      File.join(@config.output, theme, scene, file)
    end

    def convert_poster(source, target)
      command = ['ffmpeg', '-nostdin', '-hide_banner', '-loglevel', 'error', '-y',
                 '-i', source, '-frames:v', '1', '-q:v', '2', target]
      raise ArgumentError, "poster conversion failed: #{source}" unless system(*command)
    end
  end
end
