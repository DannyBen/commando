# frozen_string_literal: true

require 'cgi'
require 'digest'
require 'json'

# Version local asset URLs without renaming files or changing external URLs.
class SiteAssets
  ORIGIN = 'https://commando.sector-seven.com/'
  MANIFEST = /(<script id="asset-versions" type="application\/json">).*?(<\/script>)/m

  def initialize(root)
    @root = File.expand_path(root)
  end

  def run
    index = File.join(@root, 'index.html')
    original = File.read(index)
    raise ArgumentError, 'missing asset-versions script in site index' unless original.match?(MANIFEST)

    versions = Dir.glob('{*.css,*.js,*.ico,assets/**/*}', base: @root).sort.filter_map do |path|
      file = File.join(@root, path)
      [path, Digest::SHA256.file(file).hexdigest[0, 12]] if File.file?(file)
    end.to_h
    paths = Regexp.union(versions.keys)
    html = original.gsub(/(["'])(#{Regexp.escape(ORIGIN)}|\/)?(#{paths})([?#][^"']*)?\1/) do
      quote, prefix, path, suffix = Regexp.last_match.captures
      url = version_url(CGI.unescapeHTML("#{prefix}#{path}#{suffix}"), versions.fetch(path))
      "#{quote}#{CGI.escapeHTML(url)}#{quote}"
    end
    html.sub!(MANIFEST) { "#{Regexp.last_match(1)}#{JSON.generate(versions)}#{Regexp.last_match(2)}" }
    File.write(index, html) unless html == original
    puts "Site asset versions ready (#{versions.length} files)"
    versions
  end

  private

  def version_url(url, version)
    address, fragment = url.split('#', 2)
    path, query = address.split('?', 2)
    parameters = query.to_s.split('&').reject { |part| part.split('=', 2).first == 'v' }
    parameters << "v=#{version}"
    "#{path}?#{parameters.join('&')}#{fragment && "##{fragment}"}"
  end
end

SiteAssets.new(File.expand_path('../site', __dir__)).run if $PROGRAM_NAME == __FILE__
