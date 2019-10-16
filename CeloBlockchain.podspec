require 'json'

package = JSON.parse(File.read(File.join(__dir__, 'package.json')))

Pod::Spec.new do |s|
  s.name            = 'CeloBlockchain'
  s.version         = package['version']
  s.license         = { :type => 'BSD' }
  s.homepage        = 'https://celo.org'
  s.authors         = { 'Connor McEwen' => 'c@celo.org' }
  s.summary         = 'The Celo blockchain built for ios'
  s.source          = { :git => 'https://github.com/celo-org/celo-blockchain.git', :tag => s.version.to_s }
  s.source_files    = 'build/bin/Geth.framework/**/*.h', 'Empty.m'
  s.vendored_libraries  = 'libGeth.a', 'vendor/github.com/celo-org/bls-zexe/bls/target/universal/release/libbls_zexe.a'
  s.prepare_command     = 'touch Empty.m && ln -sf build/bin/Geth.framework/Versions/A/Geth libGeth.a'
  s.pod_target_xcconfig = { 'OTHER_LDFLAGS' => '-ObjC' }
end
