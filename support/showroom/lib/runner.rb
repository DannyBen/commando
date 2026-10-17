# frozen_string_literal: true

require 'json'
require 'fileutils'
require_relative 'config'
require_relative 'scenario'

module Showroom
  # Builds the private Go capture harness and runs each configured theme.
  class Runner
    def initialize(root)
      @root = File.expand_path(root)
      @project = File.expand_path('../..', @root)
      @config = Config.new(File.join(@root, 'config/showroom.yml'))
    end

    def run(scene)
      puts 'Preparing workspace and scene…'
      path = File.expand_path(scene, @root)
      scenario = Scenario.new(path)
      name = File.basename(path, '.rb')
      build = File.join(@config.output, 'build')
      FileUtils.mkdir_p(build)
      binary = build_renderer(build)
      @config.capture.fetch('themes').each do |theme|
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
      request = @config.payload.merge(theme: theme, output: output, steps: scenario.steps)
      path = File.join(build, "#{name}-#{theme}.json")
      File.write(path, JSON.pretty_generate(request))
      path
    end

    # Child processes inherit the console; the heartbeat stays visible while a
    # compiler or graphics call runs without producing its own output.
    def progress(label)
      started = Process.clock_gettime(Process::CLOCK_MONOTONIC)
      puts "#{label}…"
      heartbeat = progress_heartbeat(label, started)
      yield
      report_progress(label, started, 'finished')
    ensure
      heartbeat&.kill
      heartbeat&.join
    end

    def progress_heartbeat(label, started)
      Thread.new do
        loop do
          sleep 5
          report_progress(label, started, '— still working')
        end
      end
    end

    def report_progress(label, started, status)
      elapsed = Process.clock_gettime(Process::CLOCK_MONOTONIC) - started
      puts format('%<label>s %<status>s (%<elapsed>.1fs)', label: label, status: status, elapsed: elapsed)
    end
  end
end
