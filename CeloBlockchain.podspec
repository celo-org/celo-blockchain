Pod::Spec.new do |s|
  s.name             = 'CeloBlockchain'
  s.version         = '0.0.1'
  s.license         =  { :type => 'BSD' }
  s.homepage         = 'http://celo.org'
  s.authors         = { 'Connor McEwen' => 'c@celo.org' }
  s.summary         = 'The Celo blockchain built for ios'
  s.source = { :git => 'https://github.com/celo-org/celo-blockchain.git', :tag => s.version.to_s }
  s.vendored_frameworks = 'build/bin/Geth.framework'
  s.vendored_libraries = 'vendor/github.com/celo-org/bls-zexe/bls/target/universal/release/libbls_zexe.a'
  s.libraries = 'bls_zexe'
  s.static_framework = true
end
