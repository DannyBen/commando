# frozen_string_literal: true

require 'minitest/autorun'
require 'tmpdir'
require 'fileutils'
require 'open3'
require 'json'
require 'yaml'
require 'zlib'

# Exercises the real CLI and FFmpeg with tiny PNGs and no Go executable on PATH.
class VideoTest < Minitest::Test
  ROOT = File.expand_path('..', __dir__)

  def setup
    @root = Dir.mktmpdir('showroom-video-test')
    @output = File.join(@root, 'out/light/workspace')
    FileUtils.mkdir_p([@output, File.join(@root, 'config/movies'), File.join(@root, 'bin')])
    write_config
    write_png('first.png', [255, 0, 0])
    write_png('second.png', [0, 255, 0])
    File.write(File.join(@root, 'config/movies/workspace.rb'),
               "shot 'first', duration: 0.2\nshot 'second', duration: 0.3\n")
    File.symlink(executable('ffmpeg'), File.join(@root, 'bin/ffmpeg'))
  end

  def teardown
    FileUtils.remove_entry(@root)
  end

  def executable(name)
    ENV.fetch('PATH').split(File::PATH_SEPARATOR).map { |dir| File.join(dir, name) }.find do |path|
      File.executable?(path)
    end || raise("#{name} is required for video integration tests")
  end

  def write_config
    data = YAML.safe_load_file(File.join(ROOT, 'config/showroom.yml'))
    data['workspace'] = File.join(ROOT, 'config/workspace.yml')
    data['capture']['themes'] = ['light']
    data['video']['fps'] = 10
    File.write(File.join(@root, 'config/showroom.yml'), YAML.dump(data))
  end

  def write_png(name, rgb, width: 17)
    header = [width, 15, 8, 2, 0, 0, 0].pack('NNC5')
    pixels = ("\0".b + rgb.pack('C3') * width) * 15
    png = "\x89PNG\r\n\x1a\n".b + chunk('IHDR', header) + chunk('IDAT', Zlib.deflate(pixels)) + chunk('IEND', '')
    File.binwrite(File.join(@output, name), png)
  end

  def chunk(type, data)
    [data.bytesize].pack('N') + type + data + [Zlib.crc32(type + data)].pack('N')
  end

  def cli(*args)
    env = { 'PATH' => File.join(@root, 'bin') }
    code = 'exit Showroom::CLI.run(ARGV.drop(1), ARGV.first)'
    Open3.capture3(env, RbConfig.ruby, '-r', File.join(ROOT, 'lib/cli'), '-e', code, @root, *args)
  end

  def test_video_command_works_without_go_or_capture_scenes
    stdout, stderr, status = cli('vid')
    assert status.success?, stderr
    refute_includes stdout, 'renderer'
    assert_video('master.mkv', 'ffv1', 17, 15)
    assert_video('preview.mp4', 'h264', 18, 16)
    assert_lossless_cut
  end

  def assert_video(name, codec, width, height)
    json, status = Open3.capture2(executable('ffprobe'), '-v', 'error', '-count_frames',
                                  '-show_streams', '-show_format', '-of', 'json', File.join(@output, name))
    assert status.success?
    data = JSON.parse(json)
    stream = data.fetch('streams').fetch(0)
    assert_equal [codec, width, height, '5', '10/1'],
                 stream.values_at('codec_name', 'width', 'height', 'nb_read_frames', 'r_frame_rate')
    assert_in_delta 0.5, data.fetch('format').fetch('duration').to_f, 0.001
  end

  def assert_lossless_cut
    pixels, status = Open3.capture2(executable('ffmpeg'), '-v', 'error', '-i', File.join(@output, 'master.mkv'),
                                    '-f', 'rawvideo', '-pix_fmt', 'rgb24', '-', binmode: true)
    assert status.success?
    assert_equal ([255, 0, 0].pack('C3') * (17 * 15 * 2) + [0, 255, 0].pack('C3') * (17 * 15 * 3)), pixels
  end

  def test_missing_screenshot_fails_before_encoding
    File.unlink(File.join(@output, 'second.png'))
    _stdout, stderr, status = cli('vid')
    refute status.success?
    assert_includes stderr, 'showroom shot workspace first'
    refute File.exist?(File.join(@output, 'master.mkv'))
  end

  def test_mixed_dimensions_are_rejected
    write_png('second.png', [0, 255, 0], width: 18)
    _stdout, stderr, status = cli('vid')
    refute status.success?
    assert_includes stderr, 'different dimensions'
  end

  def test_help_needs_no_configuration
    File.unlink(File.join(@root, 'config/showroom.yml'))
    stdout, _stderr, status = cli('--help')
    assert status.success?
    assert_includes stdout, 'showroom vid [SCENE]'
  end

  def test_unknown_command_has_nonzero_exit_status
    _stdout, _stderr, status = cli('unknown')
    refute status.success?
  end
end
