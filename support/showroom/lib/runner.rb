# frozen_string_literal: true

require 'json'
require 'fileutils'
require_relative 'config'
require_relative 'scenario'
require_relative 'progress'

module Showroom
  # Builds the private Go capture harness and runs each configured theme.
  class Runner
    include Progress

    def initialize(root)
      @root = File.expand_path(root)
      @project = File.expand_path('../..', @root)
      @config = Config.new(File.join(@root, 'config/showroom.yml'))
    end

    def run(scene)
      puts 'Preparing workspace and scene…'
      path = File.expand_path(scene, @root)
      scenarios = @config.capture.fetch('themes').to_h { |theme| [theme, Scenario.new(path, theme: theme)] }
      name = File.basename(path, '.rb')
      build = File.join(@config.output, 'build')
      FileUtils.mkdir_p(build)
      binary = build_renderer(build)
      scenarios.each do |theme, scenario|
        capture(binary, build, name, theme, scenario)
      end
    end

    private

    def build_renderer(build)
      overlay = renderer_overlay(build)
      binary = File.join(build, 'screenshotter')
      command = ['go', 'test', '-c', '-tags=showroom,novulkan,nox11', '-overlay', overlay,
                 '-o', binary, './internal/app']
      progress('Checking/building renderer') do
        raise ArgumentError, 'renderer build failed' unless system(*command, chdir: @project)
      end
      binary
    end

    def renderer_overlay(build)
      overlay = File.join(build, 'overlay.json')
      target = File.join(@project, 'internal/app/showroom_screenshotter_test.go')
      source = File.join(@root, 'tools/screenshotter_test.go')
      File.write(overlay, JSON.pretty_generate('Replace' => { target => source }))
      overlay
    end

    def capture(binary, build, name, theme, scenario)
      output = File.join(@config.output, theme, name)
      FileUtils.mkdir_p(output)
      request_path = capture_request(build, name, theme, output, scenario)
      env = { 'COMMANDO_SHOWROOM_REQUEST' => request_path,
              'EGL_PLATFORM' => ENV.fetch('EGL_PLATFORM', 'surfaceless') }
      progress("Capturing #{name} (#{theme})") do
        command = [binary, '-test.run=^TestShowroomScreenshots$', '-test.v']
        raise ArgumentError, "capture failed: #{name} (#{theme})" unless system(env, *command, chdir: @project)
      end
    end

    def capture_request(build, name, theme, output, scenario)
      capture = @config.capture.merge(scenario.dimensions)
      request = @config.payload.merge(capture: capture, theme: theme, output: output, steps: scenario.steps)
      path = File.join(build, "#{name}-#{theme}.json")
      File.write(path, JSON.pretty_generate(request))
      path
    end
  end
end
