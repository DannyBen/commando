# frozen_string_literal: true

require 'minitest/autorun'
require_relative '../lib/cli'

class CLITest < Minitest::Test
  STEPS = [[:shot, 'config/scenes/overview.rb'],
           [:shot, 'config/scenes/panels.rb'], [:vid, 'panels'],
           [:shot, 'config/scenes/themes.rb'], [:vid, 'themes'],
           [:shot, 'config/scenes/commands.rb'], [:vid, 'commands'],
           [:publish]].freeze

  # Record the real CLI's stage order without recapturing or publishing media.
  Stage = Struct.new(:calls, :kind, :failure) do
    def run(scene = nil)
      event = [kind, scene].compact
      calls << event
      raise ArgumentError, 'stage failed' if event == failure
    end
  end

  def with_stage(klass, stage)
    original = klass.method(:new)
    klass.define_singleton_method(:new) { |_root| stage }
    yield
  ensure
    klass.define_singleton_method(:new, original)
  end

  def with_stages(failure = nil)
    calls = []
    with_stage(Showroom::Runner, Stage.new(calls, :shot, failure)) do
      with_stage(Showroom::Video, Stage.new(calls, :vid, failure)) do
        with_stage(Showroom::Publish, Stage.new(calls, :publish, failure)) { yield calls }
      end
    end
  end

  def test_all_captures_and_encodes_site_scenes_before_publishing
    with_stages do |calls|
      assert_equal 0, Showroom::CLI.run(['all'], '/showroom')
      assert_equal STEPS, calls
    end
  end

  def test_all_stops_on_capture_or_video_failure_without_publishing
    [STEPS[0], STEPS[4]].each do |failure|
      with_stages(failure) do |calls|
        error = assert_raises(ArgumentError) { Showroom::CLI.run(['all'], '/showroom') }
        assert_equal 'stage failed', error.message
        assert_equal STEPS.take(STEPS.index(failure) + 1), calls
      end
    end
  end
end
