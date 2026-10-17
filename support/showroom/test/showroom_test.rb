# frozen_string_literal: true

require 'minitest/autorun'
require 'tempfile'
require_relative '../lib/config'
require_relative '../lib/scenario'
require_relative '../lib/timeline'

# Contract tests for scene authoring and showcase data, without building Go.
class ShowroomTest < Minitest::Test
  CONFIG = File.expand_path('../config/showroom.yml', __dir__)
  WORKSPACE = File.expand_path('../config/workspace.yml', __dir__)

  def with_file(content)
    Tempfile.create('showroom-test') do |file|
      file.write(content)
      file.flush
      yield file.path
    end
  end

  def test_workspace_expands_nested_folders_and_inherits_dates
    folders = Showroom::Workspace.new(WORKSPACE).folders
    file = folders.fetch('C:\Projects\commando').find { |item| item[:name] == 'main.go' }
    assert_equal 3200, file[:bytes]
    assert_equal '2026-10-01T12:00:00Z', file['modified']
    assert_empty folders.fetch('C:\Projects\commando\assets')
  end

  def test_duplicate_windows_names_are_rejected
    data = { 'roots' => { 'C:\Demo' => { 'files' => [{ 'name' => 'a.txt' }, { 'name' => 'A.txt' }] } } }
    with_file(YAML.dump(data)) do |path|
      assert_raises(ArgumentError) { Showroom::Workspace.new(path) }
    end
  end

  def test_config_paths_are_relative_to_config_file
    config = Showroom::Config.new(CONFIG)
    assert_equal File.expand_path('../out', File.dirname(CONFIG)), config.output
    assert_equal 960, config.capture.fetch('width')
    assert_equal 3, config.panels.length
  end

  def test_unknown_selection_is_rejected
    data = YAML.safe_load_file(CONFIG)
    data['workspace'] = WORKSPACE
    data['panels'][0]['selected'] = 'missing-folder'
    with_file(YAML.dump(data)) do |path|
      assert_raises(ArgumentError) { Showroom::Config.new(path) }
    end
  end

  def test_scene_preserves_action_order_and_repetition
    with_file("press 'Down', times: 2\ntype 'panel'\nwait 400\nscreenshot 'done'\n") do |path|
      assert_equal [{ key: 'Down' }, { key: 'Down' }, { text: 'panel' },
                    { wait_ms: 400 }, { file: 'done.png' }], Showroom::Scenario.new(path).steps
    end
  end

  def test_scene_can_choose_actions_for_the_capture_theme
    with_file("press(theme == 'dark' ? 'Home' : 'End')\nscreenshot 'done'") do |path|
      assert_equal({ key: 'Home' }, Showroom::Scenario.new(path, theme: 'dark').steps.first)
      assert_equal({ key: 'End' }, Showroom::Scenario.new(path, theme: 'light').steps.first)
    end
  end

  def test_scene_can_set_its_own_canvas_size
    with_file("size width: 960, height: 370\nscreenshot 'overview'") do |path|
      assert_equal({ 'width' => 960, 'height' => 370 }, Showroom::Scenario.new(path).dimensions)
    end
  end

  def test_invalid_canvas_size_is_rejected
    with_file("size width: 960, height: 0\nscreenshot 'overview'") do |path|
      assert_raises(ArgumentError) { Showroom::Scenario.new(path) }
    end
  end

  def test_density_changes_pixels_without_changing_logical_dimensions
    with_file("size width: 960, height: 540\ndensity 2\nscreenshot 'dense'") do |path|
      assert_equal({ 'width' => 960, 'height' => 540, 'scale' => 2 }, Showroom::Scenario.new(path).dimensions)
    end
    with_file("density 2\nsize width: 960, height: 540\nscreenshot 'dense'") do |path|
      assert_equal({ 'width' => 960, 'height' => 540, 'scale' => 2 }, Showroom::Scenario.new(path).dimensions)
    end
  end

  def test_invalid_density_is_rejected
    [0, 4, 1.5].each do |scale|
      with_file("density #{scale}\nscreenshot 'dense'") do |path|
        assert_raises(ArgumentError) { Showroom::Scenario.new(path) }
      end
    end
  end

  def test_unsafe_and_duplicate_screenshot_names_are_rejected
    ["screenshot '../outside'", "screenshot 'same'\nscreenshot 'same'"].each do |scene|
      with_file(scene) { |path| assert_raises(ArgumentError) { Showroom::Scenario.new(path) } }
    end
  end

  def test_negative_time_is_rejected
    with_file("wait -1\nscreenshot 'done'") do |path|
      assert_raises(ArgumentError) { Showroom::Scenario.new(path) }
    end
  end

  def test_timeline_preserves_order_and_rounds_to_frames
    with_file("shot 'first', duration: 0.25\nshot 'second', duration: 0.5") do |path|
      timeline = Showroom::Timeline.new(path, 30)
      assert_equal [{ file: 'first.png', frames: 8 }, { file: 'second.png', frames: 15 }], timeline.shots
      assert_equal 23, timeline.frames
    end
  end

  def test_invalid_timeline_durations_are_rejected
    ['0', '-1', 'Float::INFINITY', '0.001'].each do |duration|
      with_file("shot 'first', duration: #{duration}") do |path|
        assert_raises(ArgumentError) { Showroom::Timeline.new(path, 30) }
      end
    end
  end
end
