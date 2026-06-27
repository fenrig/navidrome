import {
  buildSmartPlaylistRules,
  cleanPlaylistPayload,
} from './smartPlaylistRulesUtils'

describe('smartPlaylistRulesUtils', () => {
  it('builds automatic playlist criteria from common fields', () => {
    expect(
      buildSmartPlaylistRules({
        smartMode: true,
        smartLovedOnly: true,
        smartTitle: 'Ozora, Psy-Fi',
        smartGenre: 'Psytrance, Downtempo',
        smartArtist: 'Larry Heard',
        smartBpmMin: 120,
        smartBpmMax: 126,
        smartTagField: 'label',
        smartTagValue: 'Alleviated',
        smartSort: 'random',
        smartOrder: 'asc',
        smartLimit: 50,
      }),
    ).toEqual({
      all: [
        { is: { loved: true } },
        { contains: { title: 'Ozora' } },
        { contains: { title: 'Psy-Fi' } },
        {
          any: [
            { contains: { genre: 'Psytrance' } },
            { contains: { genre: 'Downtempo' } },
          ],
        },
        { contains: { artist: 'Larry Heard' } },
        { inTheRange: { bpm: [120, 126] } },
        { contains: { label: 'Alleviated' } },
      ],
      sort: 'random',
      order: 'asc',
      limit: 50,
    })
  })

  it('removes form-only fields before saving', () => {
    expect(
      cleanPlaylistPayload({
        id: 'playlist-id',
        name: 'Favorites',
        smartMode: true,
        smartLovedOnly: true,
        smartSort: 'dateadded',
        smartOrder: 'desc',
      }),
    ).toEqual({
      id: 'playlist-id',
      name: 'Favorites',
      rules: {
        all: [{ is: { loved: true } }],
        sort: 'dateadded',
        order: 'desc',
      },
      evaluatedAt: null,
    })
  })

  it('uses BPM presets before manual BPM values', () => {
    expect(
      buildSmartPlaylistRules({
        smartMode: true,
        smartBpmPreset: 'trance',
        smartBpmMin: 80,
        smartBpmMax: 100,
      }),
    ).toEqual({
      all: [{ inTheRange: { bpm: [130, 144] } }],
      sort: 'random',
      order: 'asc',
    })
  })

  it('supports open-ended BPM presets', () => {
    expect(
      buildSmartPlaylistRules({
        smartMode: true,
        smartBpmPreset: 'fast',
      }),
    ).toEqual({
      all: [{ gte: { bpm: 145 } }],
      sort: 'random',
      order: 'asc',
    })
  })

  it('preserves existing smart rules when defaults are not included in form values', () => {
    expect(
      cleanPlaylistPayload({
        name: 'Existing smart playlist',
        rules: {
          all: [{ contains: { genre: 'Jazz' } }],
          sort: 'title',
          order: 'asc',
        },
      }),
    ).toEqual({
      name: 'Existing smart playlist',
      rules: {
        all: [{ contains: { genre: 'Jazz' } }],
        sort: 'title',
        order: 'asc',
      },
      evaluatedAt: null,
    })
  })
})
