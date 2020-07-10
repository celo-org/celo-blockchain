require 'json'

package = JSON.parse(File.read(File.join(__dir__, 'package.json')))

Pod::Spec.new do |s|
  s.name            = 'CeloBlockchain'
  s.version         = package['version']
  s.license         = package['license']
  s.homepage        = package['homepage']
  s.authors         = { 'Connor McEwen' => 'c@celo.org' }
  s.summary         = package['description']
  s.source          = { :git => package['repository']['url'], :tag => s.version }
  s.source_files    = 'build/bin/Geth.framework/**/*.h', 'Empty.m'
  s.vendored_libraries  = 'libGeth.a', 'libbls_snark_sys.a'
  s.pod_target_xcconfig = { 'OTHER_LDFLAGS' => '-ObjC' }
end
