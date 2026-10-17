# frozen_string_literal: true

module Showroom
  # Each DSL call emits an ordered instruction; only screenshot captures a frame.
  class Scenario
    attr_reader :steps

    def initialize(path)
      @steps = []
      @screenshots = []
      instance_eval(File.read(path), path, 1)
      raise ArgumentError, 'scene needs at least one screenshot' if @screenshots.empty?
    end

    def press(key, times: 1)
      raise ArgumentError, 'times must be a positive integer' unless times.is_a?(Integer) && times.positive?

      times.times { steps << { key: key } }
    end

    def type(text)
      steps << { text: text }
    end

    # Milliseconds of UI time, without sleeping in real time.
    def wait(milliseconds)
      unless milliseconds.is_a?(Integer) && milliseconds >= 0
        raise ArgumentError, 'wait requires nonnegative integer milliseconds'
      end

      steps << { wait_ms: milliseconds }
    end

    def screenshot(name)
      unless name.is_a?(String) && name.match?(/\A[\w-]+\z/) && !@screenshots.include?(name)
        raise ArgumentError, 'screenshot names must be unique and contain only letters, digits, underscores or hyphens'
      end

      @screenshots << name
      steps << { file: "#{name}.png" }
    end
  end
end
