#[macro_use]
extern crate criterion;

use criterion::{Criterion, ParameterizedBenchmark};

use algebra::{
    bls12_377::{G2Projective, Parameters as Bls12_377Parameters},
    ProjectiveCurve,
};

use bls_crypto::curve::cofactor::{scale_by_cofactor_fuentes, scale_by_cofactor_scott};

use rand::{Rng, SeedableRng};
use rand_xorshift::XorShiftRng;

fn bench_scale_by_cofactor(c: &mut Criterion) {
    let mut rng = XorShiftRng::from_seed([
        0x5d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc, 0x06,
        0x54,
    ]);
    let mut points: Vec<G2Projective> = vec![];
    const SAMPLES: usize = 3;
    for _i in 0..SAMPLES {
        points.push(rng.gen());
    }
    /*
    let cofactor_naive = Fun::new("cofactor_naive", |b, _| {
        b.iter(|| {
            let p: G2Projective = rng.gen();
            p.into_affine().scale_by_cofactor();
        });
    });

    let cofactor_fast = Fun::new("cofactor_fast", |b, _| {
        let mut rng = XorShiftRng::from_seed([0x5dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        b.iter(|| {
            let p: G2Projective = rng.gen();
            scale_by_cofactor_fast::<Bls12_377Parameters>(&p);
        });

    });
    c.bench_functions("cofactor", vec![cofactor_naive, cofactor_fast], 0);
    */
    c.bench(
        "cofactor",
        ParameterizedBenchmark::new(
            "naive",
            |b, i| {
                b.iter(|| {
                    (*i).into_affine().scale_by_cofactor();
                })
            },
            points,
        )
        .with_function("scott", |b, i| {
            b.iter(|| {
                scale_by_cofactor_scott::<Bls12_377Parameters>(i);
            })
        })
        .with_function("fuentes", |b, i| {
            b.iter(|| {
                scale_by_cofactor_fuentes::<Bls12_377Parameters>(i);
            })
        }),
    );
}

criterion_group!(benches, bench_scale_by_cofactor);
criterion_main!(benches);
