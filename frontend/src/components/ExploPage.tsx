import { useState, useEffect, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { Spinner } from '@/components/ui/spinner';
import { toastWithSound as toast } from '@/lib/toast-with-sound';
import {
  Sparkles,
  Compass,
  Radio,
  Download,
  ListPlus,
  Play,
  Pause,
  Trash2,
  Search,
  ExternalLink,
  CheckSquare,
  Square,
  FileDown,
  Info,
} from 'lucide-react';
import type { TrackMetadata } from '@/types/api';
import { addCollectionToQueue } from '@/lib/queue';

interface ExploPageProps {
  onDownloadTracks: (tracks: TrackMetadata[], folderName: string) => void;
  onNavigateToQueue: () => void;
}

interface ExploTrackItem {
  title: string;
  artist: string;
  album?: string;
  recording_mbid?: string;
  spotify_id?: string;
  duration_ms?: number;
  image_url?: string;
  release_date?: string;
  preview_url?: string;
}

interface ExploPlaylistData {
  id: string;
  title: string;
  description: string;
  creator: string;
  track_count: number;
  tracks: ExploTrackItem[];
}

type RecommendationMode =
  | 'exploration'
  | 'jams'
  | 'daily'
  | 'artist_radio'
  | 'custom_url';

export function ExploPage({
  onDownloadTracks,
  onNavigateToQueue,
}: ExploPageProps) {
  const [username, setUsername] = useState(() => {
    return localStorage.getItem('spotiflac_listenbrainz_username') || '';
  });
  const [activeMode, setActiveMode] =
    useState<RecommendationMode>('exploration');
  const [radioQuery, setRadioQuery] = useState('');
  const [customUrl, setCustomUrl] = useState('');

  const [isLoading, setIsLoading] = useState(false);
  const [loadingStage, setLoadingStage] = useState('');
  const [playlist, setPlaylist] = useState<ExploPlaylistData | null>(null);
  const [resolvedTracks, setResolvedTracks] = useState<TrackMetadata[]>([]);
  const [selectedTrackIds, setSelectedTrackIds] = useState<Set<string>>(
    new Set(),
  );
  const [filterQuery, setFilterQuery] = useState('');

  // Audio preview state
  const [playingPreviewId, setPlayingPreviewId] = useState<string | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  useEffect(() => {
    if (username.trim()) {
      localStorage.setItem('spotiflac_listenbrainz_username', username.trim());
    }
  }, [username]);

  const handleModeChange = (mode: RecommendationMode) => {
    setActiveMode(mode);
  };

  const handleFetchRecommendations = async () => {
    setIsLoading(true);
    setLoadingStage('Connecting to ListenBrainz...');
    try {
      const app = (window as any)?.go?.main?.App;
      if (!app) {
        throw new Error('Wails backend App is not available');
      }

      let resPlaylist: ExploPlaylistData | null = null;

      if (activeMode === 'custom_url') {
        if (!customUrl.trim()) {
          toast.warning('Please enter a ListenBrainz playlist URL or MBID');
          setIsLoading(false);
          return;
        }
        setLoadingStage('Fetching ListenBrainz playlist...');
        resPlaylist = await app.FetchListenBrainzPlaylistTracks(
          customUrl.trim(),
        );
      } else if (activeMode === 'artist_radio') {
        if (!radioQuery.trim()) {
          toast.warning('Please enter an artist or genre name for Radio');
          setIsLoading(false);
          return;
        }
        setLoadingStage(
          `Generating ${radioQuery.trim()} Radio recommendations...`,
        );
        resPlaylist = await app.FetchListenBrainzRecommendations(
          '',
          'artist_radio',
          radioQuery.trim(),
        );
      } else {
        // exploration, jams, daily
        if (!username.trim()) {
          toast.warning(
            'Please enter your ListenBrainz username to load recommendations',
          );
          setIsLoading(false);
          return;
        }
        const recType = activeMode === 'jams' ? 'jams' : 'exploration';
        setLoadingStage(`Fetching recommendations for ${username.trim()}...`);
        resPlaylist = await app.FetchListenBrainzRecommendations(
          username.trim(),
          recType,
          '',
        );
      }

      if (
        !resPlaylist ||
        !resPlaylist.tracks ||
        resPlaylist.tracks.length === 0
      ) {
        throw new Error('No tracks were found for this recommendation request');
      }

      setPlaylist(resPlaylist);
      setLoadingStage(
        `Resolving ${resPlaylist.tracks.length} tracks to Spotify & MusicBrainz metadata...`,
      );

      // Resolve rich Spotify metadata (cover art, IDs, duration)
      const tracksMeta: TrackMetadata[] = await app.ResolveExploTracksToSpotify(
        resPlaylist.tracks,
      );
      setResolvedTracks(tracksMeta);

      // Default: select all resolved tracks
      const initialSelected = new Set(
        tracksMeta.map((t, idx) => t.spotify_id || `explo-${idx}`),
      );
      setSelectedTrackIds(initialSelected);

      toast.success(
        `Generated "${resPlaylist.title}" with ${tracksMeta.length} tracks!`,
      );
    } catch (err) {
      console.error('Explo recommendation error:', err);
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
      setLoadingStage('');
    }
  };

  const toggleTrackSelection = (id: string) => {
    setSelectedTrackIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const selectAll = () => {
    const all = new Set(
      resolvedTracks.map((t, idx) => t.spotify_id || `explo-${idx}`),
    );
    setSelectedTrackIds(all);
  };

  const deselectAll = () => {
    setSelectedTrackIds(new Set());
  };

  const removeTrack = (indexToRemove: number) => {
    setResolvedTracks((prev) => prev.filter((_, idx) => idx !== indexToRemove));
  };

  const handleTogglePreview = (previewUrl?: string, id?: string) => {
    if (!previewUrl || !id) return;
    if (playingPreviewId === id) {
      audioRef.current?.pause();
      setPlayingPreviewId(null);
    } else {
      if (!audioRef.current) {
        audioRef.current = new Audio();
        audioRef.current.onended = () => setPlayingPreviewId(null);
      }
      audioRef.current.src = previewUrl;
      audioRef.current.play().catch(() => {});
      setPlayingPreviewId(id);
    }
  };

  const getSelectedTracks = (): TrackMetadata[] => {
    return resolvedTracks.filter((t, idx) => {
      const id = t.spotify_id || `explo-${idx}`;
      return selectedTrackIds.has(id);
    });
  };

  const handleDownloadSelected = () => {
    const selected = getSelectedTracks();
    if (selected.length === 0) {
      toast.warning('No tracks selected for download');
      return;
    }
    const folderName = playlist?.title || 'Explo Discovery';
    onDownloadTracks(selected, folderName);
    toast.success(`Added ${selected.length} track(s) to download queue!`);
    onNavigateToQueue();
  };

  const handleAddToQueueOnly = () => {
    const selected = getSelectedTracks();
    if (selected.length === 0) {
      toast.warning('No tracks selected');
      return;
    }
    const folderName = playlist?.title || 'Explo Discovery';
    addCollectionToQueue({
      type: 'playlist',
      name: folderName,
      artist: selected[0]?.artists || 'Explo Recommendations',
      info: `${selected.length} tracks`,
      image: selected[0]?.images || '',
      folderName,
      tracks: selected,
    });
    toast.success(`Queued ${selected.length} track(s)!`);
  };

  const handleExportM3U = async () => {
    const selected = getSelectedTracks();
    if (selected.length === 0) return;
    const lines = ['#EXTM3U'];
    for (const track of selected) {
      const seconds = Math.round((track.duration_ms || 0) / 1000);
      lines.push(`#EXTINF:${seconds},${track.artists} - ${track.name}`);
      lines.push(`${track.artists} - ${track.name}.flac`);
    }
    const blob = new Blob([lines.join('\n')], { type: 'audio/x-mpegurl' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${playlist?.title || 'Explo-Playlist'}.m3u8`;
    a.click();
    URL.revokeObjectURL(url);
    toast.success('Exported M3U8 playlist file');
  };

  // Filtered tracks
  const displayedTracks = resolvedTracks.filter((track) => {
    if (!filterQuery.trim()) return true;
    const q = filterQuery.toLowerCase();
    return (
      track.name.toLowerCase().includes(q) ||
      track.artists.toLowerCase().includes(q) ||
      track.album_name.toLowerCase().includes(q)
    );
  });

  const totalSelectedDurationMs = getSelectedTracks().reduce(
    (sum, t) => sum + (t.duration_ms || 0),
    0,
  );

  const formatTotalTime = (ms: number) => {
    const mins = Math.floor(ms / 60000);
    const hrs = Math.floor(mins / 60);
    const remMins = mins % 60;
    if (hrs > 0) return `${hrs} hr ${remMins} min`;
    return `${mins} min`;
  };

  const formatTrackDuration = (ms: number) => {
    const sec = Math.floor(ms / 1000);
    const m = Math.floor(sec / 60);
    const s = sec % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  return (
    <div className='space-y-6'>
      {/* Header */}
      <div className='flex flex-col gap-2'>
        <div className='flex items-center gap-3'>
          <div className='rounded-xl bg-primary/10 p-2.5 text-primary'>
            <Sparkles className='h-6 w-6' />
          </div>
          <div>
            <h1 className='text-2xl font-bold tracking-tight'>
              Explo Music Discovery
            </h1>
            <p className='text-sm text-muted-foreground'>
              Algorithmic recommendation engine powered by ListenBrainz &
              Spotify. Review and download in lossless FLAC.
            </p>
          </div>
        </div>
      </div>

      {/* Control Card */}
      <Card>
        <CardHeader className='pb-4'>
          <CardTitle className='text-base font-semibold'>
            Recommendation Source
          </CardTitle>
          <CardDescription>
            Choose a personalized discovery algorithm or explore via
            Artist/Genre Radio.
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-5'>
          {/* Preset Buttons */}
          <div className='grid grid-cols-2 sm:grid-cols-4 gap-2.5'>
            <Button
              type='button'
              variant={activeMode === 'exploration' ? 'default' : 'outline'}
              className='flex items-center gap-2 h-auto py-3 px-4 justify-start'
              onClick={() => handleModeChange('exploration')}
            >
              <Compass className='h-4 w-4 shrink-0' />
              <div className='text-left'>
                <div className='font-semibold text-xs'>Weekly Exploration</div>
                <div className='text-[10px] opacity-75'>Discover Weekly</div>
              </div>
            </Button>

            <Button
              type='button'
              variant={activeMode === 'jams' ? 'default' : 'outline'}
              className='flex items-center gap-2 h-auto py-3 px-4 justify-start'
              onClick={() => handleModeChange('jams')}
            >
              <Radio className='h-4 w-4 shrink-0' />
              <div className='text-left'>
                <div className='font-semibold text-xs'>Weekly Jams</div>
                <div className='text-[10px] opacity-75'>Heavy Rotation</div>
              </div>
            </Button>

            <Button
              type='button'
              variant={activeMode === 'artist_radio' ? 'default' : 'outline'}
              className='flex items-center gap-2 h-auto py-3 px-4 justify-start'
              onClick={() => handleModeChange('artist_radio')}
            >
              <Sparkles className='h-4 w-4 shrink-0' />
              <div className='text-left'>
                <div className='font-semibold text-xs'>
                  Artist / Genre Radio
                </div>
                <div className='text-[10px] opacity-75'>Instant Seed Mix</div>
              </div>
            </Button>

            <Button
              type='button'
              variant={activeMode === 'custom_url' ? 'default' : 'outline'}
              className='flex items-center gap-2 h-auto py-3 px-4 justify-start'
              onClick={() => handleModeChange('custom_url')}
            >
              <ExternalLink className='h-4 w-4 shrink-0' />
              <div className='text-left'>
                <div className='font-semibold text-xs'>Playlist Import</div>
                <div className='text-[10px] opacity-75'>URL or MBID</div>
              </div>
            </Button>
          </div>

          {/* Contextual Inputs */}
          {activeMode === 'artist_radio' ? (
            <div className='flex gap-2'>
              <Input
                placeholder='Enter artist or genre (e.g. Tame Impala, Daft Punk, Synthwave, Post-Punk)...'
                value={radioQuery}
                onChange={(e) => setRadioQuery(e.target.value)}
                onKeyDown={(e) =>
                  e.key === 'Enter' && handleFetchRecommendations()
                }
                className='flex-1'
              />
              <Button onClick={handleFetchRecommendations} disabled={isLoading}>
                {isLoading ? (
                  <Spinner className='h-4 w-4 mr-2' />
                ) : (
                  <Sparkles className='h-4 w-4 mr-2' />
                )}
                Generate Radio
              </Button>
            </div>
          ) : activeMode === 'custom_url' ? (
            <div className='flex gap-2'>
              <Input
                placeholder='Enter ListenBrainz Playlist URL or UUID (e.g. https://listenbrainz.org/playlist/...)...'
                value={customUrl}
                onChange={(e) => setCustomUrl(e.target.value)}
                onKeyDown={(e) =>
                  e.key === 'Enter' && handleFetchRecommendations()
                }
                className='flex-1'
              />
              <Button onClick={handleFetchRecommendations} disabled={isLoading}>
                {isLoading ? (
                  <Spinner className='h-4 w-4 mr-2' />
                ) : (
                  <Download className='h-4 w-4 mr-2' />
                )}
                Import Playlist
              </Button>
            </div>
          ) : (
            <div className='space-y-3'>
              <div className='flex gap-2'>
                <Input
                  placeholder='Enter your ListenBrainz username...'
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  onKeyDown={(e) =>
                    e.key === 'Enter' && handleFetchRecommendations()
                  }
                  className='flex-1'
                />
                <Button
                  onClick={handleFetchRecommendations}
                  disabled={isLoading}
                >
                  {isLoading ? (
                    <Spinner className='h-4 w-4 mr-2' />
                  ) : (
                    <Compass className='h-4 w-4 mr-2' />
                  )}
                  Generate Playlist
                </Button>
              </div>
              <div className='flex items-center gap-1.5 text-xs text-muted-foreground'>
                <Info className='h-3.5 w-3.5 text-muted-foreground/80 shrink-0' />
                <span>
                  Explo interfaces with open-source scrobbler{' '}
                  <a
                    href='https://listenbrainz.org'
                    target='_blank'
                    rel='noreferrer'
                    className='underline hover:text-primary'
                  >
                    ListenBrainz
                  </a>{' '}
                  to generate personalized Discovery recommendations.
                </span>
              </div>
            </div>
          )}

          {/* Loading Indicator */}
          {isLoading && (
            <div className='flex items-center gap-3 p-4 rounded-lg bg-muted/50 border border-border'>
              <Spinner className='h-5 w-5 text-primary' />
              <div className='text-sm font-medium text-foreground'>
                {loadingStage}
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Playlist Review Section */}
      {playlist && resolvedTracks.length > 0 && (
        <Card className='border-border'>
          <CardHeader className='pb-3 border-b border-border/50'>
            <div className='flex flex-col sm:flex-row sm:items-center justify-between gap-4'>
              <div>
                <div className='flex items-center gap-2'>
                  <CardTitle className='text-xl font-bold'>
                    {playlist.title}
                  </CardTitle>
                  <Badge variant='secondary' className='font-mono text-xs'>
                    {resolvedTracks.length} tracks
                  </Badge>
                </div>
                {playlist.description && (
                  <CardDescription className='mt-1 text-xs'>
                    {playlist.description}
                  </CardDescription>
                )}
                <div className='flex items-center gap-4 text-xs text-muted-foreground mt-2'>
                  <span>
                    Selected: <strong>{selectedTrackIds.size}</strong> of{' '}
                    {resolvedTracks.length} tracks
                  </span>
                  <span>•</span>
                  <span>
                    Duration:{' '}
                    <strong>{formatTotalTime(totalSelectedDurationMs)}</strong>
                  </span>
                </div>
              </div>

              {/* Action Toolbar */}
              <div className='flex items-center gap-2 flex-wrap'>
                <Button
                  size='sm'
                  variant='outline'
                  onClick={
                    selectedTrackIds.size === resolvedTracks.length
                      ? deselectAll
                      : selectAll
                  }
                >
                  {selectedTrackIds.size === resolvedTracks.length ? (
                    <>
                      <Square className='h-3.5 w-3.5 mr-1.5' />
                      Deselect All
                    </>
                  ) : (
                    <>
                      <CheckSquare className='h-3.5 w-3.5 mr-1.5' />
                      Select All
                    </>
                  )}
                </Button>

                <Button size='sm' variant='outline' onClick={handleExportM3U}>
                  <FileDown className='h-3.5 w-3.5 mr-1.5' />
                  M3U8
                </Button>

                <Button
                  size='sm'
                  variant='outline'
                  onClick={handleAddToQueueOnly}
                >
                  <ListPlus className='h-3.5 w-3.5 mr-1.5' />
                  Add to Queue
                </Button>

                <Button
                  size='sm'
                  onClick={handleDownloadSelected}
                  className='font-semibold shadow'
                >
                  <Download className='h-3.5 w-3.5 mr-1.5' />
                  Download Selected ({selectedTrackIds.size})
                </Button>
              </div>
            </div>

            {/* Filter Input */}
            <div className='pt-3'>
              <div className='relative'>
                <Search className='absolute left-3 top-2.5 h-4 w-4 text-muted-foreground' />
                <Input
                  placeholder='Filter tracks by title, artist, or album...'
                  value={filterQuery}
                  onChange={(e) => setFilterQuery(e.target.value)}
                  className='pl-9 h-9 text-xs'
                />
              </div>
            </div>
          </CardHeader>

          <CardContent className='p-0'>
            <div className='overflow-x-auto'>
              <table className='w-full text-left text-xs border-collapse'>
                <thead>
                  <tr className='border-b border-border/40 bg-muted/20 text-muted-foreground'>
                    <th className='py-2.5 pl-4 pr-2 w-10 text-center'>
                      <Checkbox
                        checked={
                          selectedTrackIds.size === resolvedTracks.length &&
                          resolvedTracks.length > 0
                        }
                        onCheckedChange={(checked) =>
                          checked ? selectAll() : deselectAll()
                        }
                      />
                    </th>
                    <th className='py-2.5 px-2 w-10 text-center'>#</th>
                    <th className='py-2.5 px-3'>Title & Artist</th>
                    <th className='py-2.5 px-3 hidden md:table-cell'>Album</th>
                    <th className='py-2.5 px-3 text-right'>Time</th>
                    <th className='py-2.5 pr-4 pl-2 w-14 text-center'>
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody className='divide-y divide-border/20'>
                  {displayedTracks.map((track, idx) => {
                    const id = track.spotify_id || `explo-${idx}`;
                    const isSelected = selectedTrackIds.has(id);
                    const isPlaying = playingPreviewId === id;

                    return (
                      <tr
                        key={id}
                        className={`hover:bg-muted/30 transition-colors ${
                          isSelected ? 'bg-primary/[0.02]' : 'opacity-60'
                        }`}
                      >
                        {/* Checkbox */}
                        <td className='py-2 pl-4 pr-2 text-center align-middle'>
                          <Checkbox
                            checked={isSelected}
                            onCheckedChange={() => toggleTrackSelection(id)}
                          />
                        </td>

                        {/* Number / Cover Thumbnail */}
                        <td className='py-2 px-2 text-center align-middle font-mono text-muted-foreground'>
                          {idx + 1}
                        </td>

                        {/* Track Info with Cover */}
                        <td className='py-2 px-3 align-middle'>
                          <div className='flex items-center gap-3'>
                            <div className='relative group h-10 w-10 rounded overflow-hidden bg-muted shrink-0 shadow-sm'>
                              {track.images ? (
                                <img
                                  src={track.images}
                                  alt={track.name}
                                  className='h-full w-full object-cover'
                                />
                              ) : (
                                <div className='h-full w-full flex items-center justify-center bg-muted'>
                                  <Radio className='h-4 w-4 text-muted-foreground' />
                                </div>
                              )}
                              {track.preview_url && (
                                <button
                                  type='button'
                                  onClick={() =>
                                    handleTogglePreview(track.preview_url, id)
                                  }
                                  className='absolute inset-0 bg-black/50 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity'
                                >
                                  {isPlaying ? (
                                    <Pause className='h-4 w-4 text-white' />
                                  ) : (
                                    <Play className='h-4 w-4 text-white fill-white ml-0.5' />
                                  )}
                                </button>
                              )}
                            </div>

                            <div className='min-w-0'>
                              <div className='font-medium text-foreground truncate flex items-center gap-1.5'>
                                <span>{track.name}</span>
                                {track.is_explicit && (
                                  <Badge
                                    variant='outline'
                                    className='h-4 px-1 text-[9px] font-mono leading-none bg-muted/60'
                                  >
                                    E
                                  </Badge>
                                )}
                              </div>
                              <div className='text-muted-foreground truncate text-[11px]'>
                                {track.artists}
                              </div>
                            </div>
                          </div>
                        </td>

                        {/* Album */}
                        <td className='py-2 px-3 align-middle text-muted-foreground hidden md:table-cell truncate max-w-[200px]'>
                          {track.album_name || '-'}
                        </td>

                        {/* Duration */}
                        <td className='py-2 px-3 align-middle text-right font-mono text-muted-foreground'>
                          {formatTrackDuration(track.duration_ms || 0)}
                        </td>

                        {/* Actions */}
                        <td className='py-2 pr-4 pl-2 align-middle text-center'>
                          <Button
                            type='button'
                            variant='ghost'
                            size='icon'
                            className='h-7 w-7 text-muted-foreground hover:text-destructive'
                            onClick={() => removeTrack(idx)}
                            title='Remove from playlist'
                          >
                            <Trash2 className='h-3.5 w-3.5' />
                          </Button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

