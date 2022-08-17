import React from 'react';
import PropTypes from 'prop-types';
import {
  Breadcrumb,
  ControlBar,
  Input,
  Button,
  Block,
  BlockHeader,
  BlockBody,
  BlockFooter,
  InfoBar,
  Dropdown,
  Tabs,
  Tab,
  Icon,
  Toggle,
} from '@kerberos-io/ui';
import { withRouter } from 'react-router-dom';
import { connect } from 'react-redux';
import ImageCanvas from '../../components/ImageCanvas/ImageCanvas';
import './Settings.scss';
import timezones from './timezones';
import {
  saveConfig,
  verifyHub,
  verifyPersistence,
  getConfig,
} from '../../actions/agent';

// eslint-disable-next-line react/prefer-stateless-function
class Settings extends React.Component {
  KERBEROS_VAULT = 'kstorage'; // @TODO needs to change

  KERBEROS_HUB = 's3'; // @TODO needs to change

  constructor() {
    super();
    this.state = {
      search: '',
      custom: {
        s3: {},
        kstorage: {},
        capture: {
          ipcamera: {},
        },
      },

      global: {
        s3: {},
        kstorage: {},
      },
      selectedTab: 'overview',
      mqttSuccess: false,
      mqttError: false,
      stunturnSuccess: false,
      stunturnError: false,
      generalSuccess: false,
      generalError: false,
      hubSuccess: false,
      hubError: false,
      verifyHubSuccess: false,
      verifyHubError: false,
      verifyHubErrorMessage: '',
      persistenceSuccess: false,
      persistenceError: false,
      verifyPersistenceSuccess: false,
      verifyPersistenceError: false,
      verifyPersistenceMessage: '',
      loading: false,
      loadingHub: false,
    };
    this.storageTypes = [
      {
        label: 'Kerberos Hub',
        value: this.KERBEROS_HUB,
      },
      {
        label: 'Kerberos Vault',
        value: this.KERBEROS_VAULT,
      },
    ];

    this.tags = {
      general: ['timezone', 'generic', 'general'],
      mqtt: ['mqtt', 'message broker', 'events', 'broker'],
      'stun-turn': ['webrtc', 'stun', 'turn', 'livestreaming'],
      'kerberos-hub': [
        'kerberos hub',
        'dashboard',
        'agents',
        'monitoring',
        'site',
      ],
      persistence: [
        'kerberos hub',
        'kerberos vault',
        'storage',
        'persistence',
        'gcp',
        'aws',
        'minio',
      ],
    };
    this.timezones = [];
    timezones.forEach((t) =>
      this.timezones.push({
        label: t.text,
        value: t.utc[0],
      })
    );

    this.changeValue = this.changeValue.bind(this);
    this.changeVaultValue = this.changeVaultValue.bind(this);
    this.changeS3Value = this.changeS3Value.bind(this);
    this.changeStorageType = this.changeStorageType.bind(this);
    this.changeTimezone = this.changeTimezone.bind(this);
    this.filterSettings = this.filterSettings.bind(this);
    this.saveGeneralSettings = this.saveGeneralSettings.bind(this);
    this.saveSTUNTURNSettings = this.saveSTUNTURNSettings.bind(this);
    this.saveMQTTSettings = this.saveMQTTSettings.bind(this);
    this.saveHubSettings = this.saveHubSettings.bind(this);
    this.savePersistenceSettings = this.savePersistenceSettings.bind(this);
    this.verifyPersistenceSettings = this.verifyPersistenceSettings.bind(this);
    this.verifyHubSettings = this.verifyHubSettings.bind(this);
    this.changeTab = this.changeTab.bind(this);
    this.calculateTimetable = this.calculateTimetable.bind(this);
  }

  componentDidMount() {
    const { dispatchConfig } = this.props;
    dispatchConfig();
  }

  componentDidUpdate(prevProps, prevState) {
    // const { service, container } = this.props;
    const { open } = this.state;
    if (prevState.open !== open && open) {
      // this.props.dispatchGetConfig(service);
      // Cache the current timetable
      // const { custom } = container;
      // this.calculateTimetable(custom.timetable);
    }
  }

  componentWillUnmount() {
    document.removeEventListener('keydown', this.escFunction, false);
    clearInterval(this.interval);
  }

  changeTab(tab) {
    this.setState({
      selectedTab: tab,
    });
  }

  changeValue() {
    // console.log(this);
  }

  changeVaultValue() {
    // console.log(this);
  }

  changeS3Value() {
    // console.log(this);
  }

  changeStorageType() {
    // console.log(this);
  }

  changeTimezone() {
    // console.log(this);
  }

  filterSettings() {
    // console.log(this);
  }

  saveGeneralSettings() {
    // console.log(this);
  }

  saveSTUNTURNSettings() {
    // console.log(this);
  }

  saveMQTTSettings() {
    // console.log(this);
  }

  saveHubSettings() {
    // console.log(this);
  }

  savePersistenceSettings() {
    // console.log(this);
  }

  verifyPersistenceSettings() {
    // console.log(this);
  }

  verifyHubSettings() {
    // console.log(this);
  }

  calculateTimetable(timetable) {
    this.timetable = timetable;
    for (let i = 0; i < timetable.length; i += 1) {
      const time = timetable[i];
      const { start1, start2, end1, end2 } = time;
      this.timetable[i].start1Full = this.convertSecondsToHourMinute(start1);
      this.timetable[i].start2Full = this.convertSecondsToHourMinute(start2);
      this.timetable[i].end1Full = this.convertSecondsToHourMinute(end1);
      this.timetable[i].end2Full = this.convertSecondsToHourMinute(end2);
    }
  }

  render() {
    const {
      selectedTab,
      search,
      custom,
      global,
      generalSuccess,
      generalError,
      mqttSuccess,
      mqttError,
      stunturnSuccess,
      stunturnError,
      hubSuccess,
      hubError,
      verifyHubSuccess,
      verifyHubError,
      verifyHubErrorMessage,
      persistenceSuccess,
      persistenceError,
      verifyPersistenceSuccess,
      verifyPersistenceError,
      verifyPersistenceMessage,
      loading,
      loadingHub,
      config, // New variables start here
    } = this.state;

    const snapshot = 'data:image/png;base64,';
    // Determine which section(s) to be shown, depending on the searching criteria.
    let showOverviewSection = false;
    let showRecordingSection = false;
    let showCameraSection = false;
    let showStreamingSection = false;
    let showConditionsHubSection = false;
    let showPersistenceSection = false;

    switch (selectedTab) {
      case 'all':
        showOverviewSection = true;
        showCameraSection = true;
        showRecordingSection = true;
        showStreamingSection = true;
        showConditionsHubSection = true;
        showPersistenceSection = true;
        break;
      case 'overview':
        showOverviewSection = true;
        break;
      case 'camera':
        showCameraSection = true;
        break;
      case 'recording':
        showRecordingSection = true;
        break;
      case 'streaming':
        showStreamingSection = true;
        break;
      case 'conditions':
        showConditionsHubSection = true;
        break;
      case 'persistence':
        showPersistenceSection = true;
        break;
      default:
    }

    if (search !== '' && search !== null) {
      if (this.tags) {
        const sections = Object.keys(this.tags);
        sections.forEach((section) => {
          // Find a match for the current section
          const sectionTags = this.tags[section];
          let match = false;
          sectionTags.forEach((tag) => {
            if (tag.toLowerCase().includes(search.toLowerCase())) {
              match = true;
            }
          });

          switch (section) {
            case 'overview':
              showOverviewSection = match;
              break;
            case 'camera':
              showCameraSection = match;
              break;
            case 'recording':
              showRecordingSection = match;
              break;
            case 'streaming':
              showStreamingSection = match;
              break;
            case 'conditions':
              showConditionsHubSection = match;
              break;
            case 'persistence':
              showPersistenceSection = match;
              break;
            default:
          }
        });
      }
    }

    return (
      <div id="settings">
        <Breadcrumb
          title="Settings"
          level1="Onboard your camera"
          level1Link=""
        />
        <ControlBar type="row">
          <Tabs>
            <Tab
              label="All"
              value="all"
              active={selectedTab === 'all'}
              onClick={() => this.changeTab('all')}
              icon={<Icon label="list" />}
            />
            <Tab
              label="Overview"
              value="overview"
              active={selectedTab === 'overview'}
              onClick={() => this.changeTab('overview')}
              icon={<Icon label="dashboard" />}
            />
            <Tab
              label="Camera"
              value="camera"
              active={selectedTab === 'camera'}
              onClick={() => this.changeTab('camera')}
              icon={<Icon label="cameras" />}
            />
            <Tab
              label="Recording"
              value="recording"
              active={selectedTab === 'recording'}
              onClick={() => this.changeTab('recording')}
              icon={<Icon label="refresh" />}
            />
            <Tab
              label="Streaming"
              value="streaming"
              active={selectedTab === 'streaming'}
              onClick={() => this.changeTab('streaming')}
              icon={<Icon label="livestream" />}
            />
            <Tab
              label="Conditions"
              value="conditions"
              active={selectedTab === 'conditions'}
              onClick={() => this.changeTab('conditions')}
              icon={<Icon label="activity" />}
            />
            <Tab
              label="Persistence"
              value="persistence"
              active={selectedTab === 'persistence'}
              onClick={() => this.changeTab('persistence')}
              icon={<Icon label="cloud" />}
            />
          </Tabs>
        </ControlBar>

        <div className="stats grid-container --two-columns">
          <div>
            {/* General settings block */}
            {showOverviewSection && (
              <Block>
                <BlockHeader>
                  <h4>General</h4>
                </BlockHeader>
                <BlockBody>
                  {generalSuccess && (
                    <InfoBar
                      type="success"
                      message="General settings are successfully saved."
                    />
                  )}
                  {generalError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    General settings allow you to configure your Kerberos Agents
                    on a higher level.
                  </p>
                  <Input label="key" disabled value={config.key} />

                  <Input label="camera name" value={config.name} />

                  <Dropdown
                    isRadio
                    label="Timezone"
                    placeholder="Select a timezone"
                    items={this.timezones}
                    selected={[config.timezone]}
                    shorten
                    shortenType="end"
                    shortenMaxLength={35}
                    onChange={this.changeTimezone}
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveGeneralSettings}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* General settings block */}
            {showCameraSection && (
              <Block>
                <BlockHeader>
                  <h4>Camera</h4>
                </BlockHeader>
                <BlockBody>
                  {generalSuccess && (
                    <InfoBar
                      type="success"
                      message="General settings are successfully saved."
                    />
                  )}
                  {generalError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    General settings allow you to configure your Kerberos Agents
                    on a higher level.
                  </p>
                  <Input
                    label="RTSP URL"
                    value={custom.capture.ipcamera.rtsp}
                    placeholder="The IP camera address"
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'rtsp',
                        value,
                        custom.capture.ipcamera
                      )
                    }
                  />

                  <Input
                    label="onvif xaddr"
                    value={custom.capture.ipcamera.onvif_xaddr}
                    placeholder="http://x.x.x.x/onvif/device_service"
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_xaddr',
                        value,
                        custom.capture.ipcamera
                      )
                    }
                  />

                  <Input
                    label="username"
                    value={custom.capture.ipcamera.onvif_username}
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_username',
                        value,
                        custom.capture.ipcamera
                      )
                    }
                  />

                  <Input
                    label="password"
                    value={custom.capture.ipcamera.onvif_password}
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_password',
                        value,
                        custom.capture.ipcamera
                      )
                    }
                  />

                  <hr />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveGeneralSettings}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* General settings block */}
            {showRecordingSection && (
              <Block>
                <BlockHeader>
                  <h4>Recording</h4>
                </BlockHeader>
                <BlockBody>
                  {generalSuccess && (
                    <InfoBar
                      type="success"
                      message="General settings are successfully saved."
                    />
                  )}
                  {generalError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    General settings allow you to configure your Kerberos Agents
                    on a higher level.
                  </p>
                  <div className="toggle-wrapper">
                    <Toggle
                      on={custom.capture.continuous === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'continuous',
                          event,
                          custom.capture
                        )
                      }
                    />
                    <div>
                      <span>Continuous recording</span>
                      <p>Make 24/7 or motion based recordings.</p>
                    </div>
                  </div>

                  <Input
                    label="video duration (seconds)"
                    value={custom.capture.maxlengthrecording}
                    placeholder="The maximum duration of a recording."
                    onChange={(value) =>
                      this.onUpdateNumberField(
                        'capture',
                        'maxlengthrecording',
                        value,
                        custom.capture
                      )
                    }
                  />

                  <Input
                    label="pre recording (seconds)"
                    value={custom.capture.prerecording}
                    placeholder="Seconds before an event occurred."
                    onChange={(value) =>
                      this.onUpdateNumberField(
                        'capture',
                        'prerecording',
                        value,
                        custom.capture
                      )
                    }
                  />

                  <Input
                    label="post recording (seconds)"
                    value={custom.capture.postrecording}
                    placeholder="Seconds after an event occurred."
                    onChange={(value) =>
                      this.onUpdateNumberField(
                        'capture',
                        'postrecording',
                        value,
                        custom.capture
                      )
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveGeneralSettings}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* STUN/TURN block */}
            {showStreamingSection && (
              <Block>
                <BlockHeader>
                  <h4>STUN/TURN for WebRTC</h4>
                </BlockHeader>
                <BlockBody>
                  {stunturnSuccess && (
                    <InfoBar
                      type="success"
                      message="STUN/TURN settings are successfully saved."
                    />
                  )}
                  {stunturnError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    For full-resolution livestreaming we use the concept of
                    WebRTC. One of the key capabilities is the ICE-candidate
                    feature, which allows NAT traversal using the concepts of
                    STUN/TURN.
                  </p>
                  <Input
                    label="STUN server"
                    value={global.stunuri}
                    onChange={(value) => this.changeValue('stunuri', value)}
                  />
                  <Input
                    label="TURN server"
                    value={global.turnuri}
                    onChange={(value) => this.changeValue('turnuri', value)}
                  />
                  <Input
                    label="Username"
                    value={global.turn_username}
                    onChange={(value) =>
                      this.changeValue('turn_username', value)
                    }
                  />
                  <Input
                    label="Password"
                    value={global.turn_password}
                    onChange={(value) =>
                      this.changeValue('turn_password', value)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveSTUNTURNSettings}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {showPersistenceSection && (
              <Block>
                <BlockHeader>
                  <h4>Kerberos Hub</h4>
                </BlockHeader>
                <BlockBody>
                  {loadingHub && (
                    <InfoBar
                      type="loading"
                      message="Verifying your Kerberos Hub settings."
                    />
                  )}
                  {verifyHubSuccess && (
                    <InfoBar
                      type="success"
                      message="Kerberos Hub settings are successfully verified."
                    />
                  )}
                  {verifyHubError && (
                    <InfoBar type="alert" message={verifyHubErrorMessage} />
                  )}
                  {hubSuccess && (
                    <InfoBar
                      type="success"
                      message="Kerberos Hub settings are successfully saved."
                    />
                  )}
                  {hubError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    Kerberos Agents can send heartbeats to a central{' '}
                    <a
                      href="https://doc.kerberos.io/hub/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Hub
                    </a>{' '}
                    installation. Heartbeats and other relevant information are
                    synced to Kerberos Hub to show realtime information about
                    your video landscape.
                  </p>
                  <Input
                    label="API url"
                    placeholder="The API for Kerberos Hub."
                    value={global.hub_uri}
                    onChange={(value) => this.changeValue('hub_uri', value)}
                  />
                  <Input
                    label="Public key"
                    placeholder="The public key granted to your Kerberos Hub account."
                    value={global.hub_key}
                    onChange={(value) => this.changeValue('hub_key', value)}
                  />
                  <Input
                    label="Private key"
                    placeholder="The private key granted to your Kerberos Hub account."
                    value={global.hub_private_key}
                    onChange={(value) =>
                      this.changeValue('hub_private_key', value)
                    }
                  />
                  <Input
                    label="Site"
                    value={global.hub_site}
                    placeholder="The site ID the Kerberos Agents are belonging to in Kerberos Hub."
                    onChange={(value) => this.changeValue('hub_site', value)}
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Verify Connection"
                    disabled={loadingHub}
                    type={loadingHub ? 'neutral' : 'default'}
                    onClick={this.verifyHubSettings}
                    icon="verify"
                  />
                  <Button
                    label="Save"
                    onClick={this.saveHubSettings}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* Conditions block */}
            {showConditionsHubSection && (
              <Block>
                <BlockHeader>
                  <h4>Region Of Interest</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    By defining one or more regions, motion will be tracked only
                    in the regions you have defined.
                  </p>
                  {custom.region && (
                    <ImageCanvas
                      image={snapshot}
                      polygons={custom.region.polygon}
                      rendered={false}
                      onAddRegion={this.onAddRegion}
                      onUpdateRegion={this.onUpdateRegion}
                      onDeleteRegion={this.onDeleteRegion}
                    />
                  )}
                </BlockBody>
              </Block>
            )}
          </div>

          <div>
            {showOverviewSection && (
              <Block>
                <BlockHeader>
                  <h4>MQTT</h4>
                </BlockHeader>
                <BlockBody>
                  {mqttSuccess && (
                    <InfoBar
                      type="success"
                      message="MQTT settings are successfully saved."
                    />
                  )}
                  {mqttError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    A MQTT broker is used to communicate from{' '}
                    <a
                      href="https://doc.kerberos.io/hub/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Hub
                    </a>{' '}
                    to the Kerberos Agent, to achieve for example livestreaming
                    or ONVIF (PTZ) capabilities.
                  </p>
                  <Input
                    label="Broker Uri"
                    value={global.mqtturi}
                    onChange={(value) => this.changeValue('mqtturi', value)}
                  />
                  <Input
                    label="Username"
                    value={global.mqtt_username}
                    onChange={(value) =>
                      this.changeValue('mqtt_username', value)
                    }
                  />
                  <Input
                    label="Password"
                    value={global.mqtt_password}
                    onChange={(value) =>
                      this.changeValue('mqtt_password', value)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveMQTTSettings}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* STUN/TURN block */}
            {showStreamingSection && (
              <Block>
                <BlockHeader>
                  <h4>Forwarding and transcoding</h4>
                </BlockHeader>
                <BlockBody>
                  {stunturnSuccess && (
                    <InfoBar
                      type="success"
                      message="STUN/TURN settings are successfully saved."
                    />
                  )}
                  {stunturnError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}

                  <div className="toggle-wrapper">
                    <Toggle
                      on={custom.capture.forwardwebrtc === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'forwardwebrtc',
                          event,
                          custom.capture
                        )
                      }
                    />
                    <div>
                      <span>Forwarding to WebRTC broker</span>
                      <p>Forward h264 stream through MQTT</p>
                    </div>
                  </div>

                  <div className="toggle-wrapper">
                    <Toggle
                      on={custom.capture.transcodingwebrtc === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'transcodingwebrtc',
                          event,
                          custom.capture
                        )
                      }
                    />
                    <div>
                      <span>Transcode stream</span>
                      <p>Convert stream to a lower resolution</p>
                    </div>
                  </div>

                  <Input
                    label="Downscale resolution (in % or original resolution)"
                    value={custom.capture.transcodingresolution}
                    placeholder="The % of the original resolution."
                    onChange={(value) =>
                      this.onUpdateNumberField(
                        'capture',
                        'transcodingresolution',
                        value,
                        custom.capture
                      )
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveSTUNTURNSettings}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* General settings block */}
            {showRecordingSection && (
              <Block>
                <BlockHeader>
                  <h4>Fragmented recordings</h4>
                </BlockHeader>
                <BlockBody>
                  {generalSuccess && (
                    <InfoBar
                      type="success"
                      message="General settings are successfully saved."
                    />
                  )}
                  {generalError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}

                  <div className="toggle-wrapper">
                    <Toggle
                      on={custom.capture.fragmented === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'fragmented',
                          event,
                          custom.capture
                        )
                      }
                    />
                    <div>
                      <span>Enable fragmentation</span>
                      <p>
                        Recordings can be be made fragmented. This is required
                        for HLS.
                      </p>
                    </div>
                  </div>

                  <Input
                    label="fragmented duration"
                    value={custom.capture.fragmentedduration}
                    placeholder="Duration of a single fragment."
                    onChange={(value) =>
                      this.onUpdateNumberField(
                        'capture',
                        'fragmentedduration',
                        value,
                        custom.capture
                      )
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveGeneralSettings}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* Conditions block */}
            {showConditionsHubSection && (
              <Block>
                <BlockHeader>
                  <h4>Time Of Interest</h4>
                </BlockHeader>
                <BlockBody>
                  <div className="grid-2">
                    {this.timetable && this.timetable.length > 0 && (
                      <div>
                        <h3>Time Of Interest</h3>
                        <p>
                          Only make recordings between specific time intervals
                          (based on Timezone).
                        </p>

                        <div className="toggle-wrapper">
                          <Toggle
                            on={custom.time === 'true'}
                            disabled={false}
                            onClick={(event) =>
                              this.onUpdateToggle('', 'time', event, custom)
                            }
                          />
                          <div>
                            <span>Enabled</span>
                            <p>If enabled you can specify time windows.</p>
                          </div>
                        </div>

                        <span className="time-of-interest">Sunday</span>
                        <div className="grid-4">
                          <Input
                            placeholder="00:00"
                            value={this.timetable[0].start1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                0,
                                'start1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:00"
                            value={this.timetable[0].end1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                0,
                                'end1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:01"
                            value={this.timetable[0].start2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                0,
                                'start2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="23:59"
                            value={this.timetable[0].end2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                0,
                                'end2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                        </div>

                        <span className="time-of-interest">Monday</span>
                        <div className="grid-4">
                          <Input
                            placeholder="00:00"
                            value={this.timetable[1].start1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                1,
                                'start1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:00"
                            value={this.timetable[1].end1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                1,
                                'end1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:01"
                            value={this.timetable[1].start2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                1,
                                'start2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="23:59"
                            value={this.timetable[1].end2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                1,
                                'end2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                        </div>

                        <span className="time-of-interest">Tuesday</span>
                        <div className="grid-4">
                          <Input
                            placeholder="00:00"
                            value={this.timetable[2].start1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                2,
                                'start1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:00"
                            value={this.timetable[2].end1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                2,
                                'end1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:01"
                            value={this.timetable[2].start2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                2,
                                'start2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="23:59"
                            value={this.timetable[2].end2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                2,
                                'end2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                        </div>

                        <span className="time-of-interest">Wednesday</span>
                        <div className="grid-4">
                          <Input
                            placeholder="00:00"
                            value={this.timetable[3].start1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                3,
                                'start1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:00"
                            value={this.timetable[3].end1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                3,
                                'end1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:01"
                            value={this.timetable[3].start2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                3,
                                'start2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="23:59"
                            value={this.timetable[3].end2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                3,
                                'end2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                        </div>

                        <span className="time-of-interest">Thursday</span>
                        <div className="grid-4">
                          <Input
                            placeholder="00:00"
                            value={this.timetable[4].start1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                4,
                                'start1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:00"
                            value={this.timetable[4].end1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                4,
                                'end1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:01"
                            value={this.timetable[4].start2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                4,
                                'start2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="23:59"
                            value={this.timetable[4].end2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                4,
                                'end2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                        </div>

                        <span className="time-of-interest">Friday</span>
                        <div className="grid-4">
                          <Input
                            placeholder="00:00"
                            value={this.timetable[5].start1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                5,
                                'start1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:00"
                            value={this.timetable[5].end1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                5,
                                'end1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:01"
                            value={this.timetable[5].start2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                5,
                                'start2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="23:59"
                            value={this.timetable[5].end2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                5,
                                'end2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                        </div>

                        <span className="time-of-interest">Saturday</span>
                        <div className="grid-4">
                          <Input
                            placeholder="00:00"
                            value={this.timetable[6].start1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                6,
                                'start1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:00"
                            value={this.timetable[6].end1Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                6,
                                'end1',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="12:01"
                            value={this.timetable[6].start2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                6,
                                'start2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                          <Input
                            placeholder="23:59"
                            value={this.timetable[6].end2Full}
                            onChange={(event) => {
                              this.onUpdateTimeline(
                                'timetable',
                                6,
                                'end2',
                                event,
                                custom.timetable
                              );
                            }}
                          />
                        </div>
                      </div>
                    )}
                  </div>
                </BlockBody>
              </Block>
            )}

            {/* Conditions block */}
            {showConditionsHubSection && (
              <Block>
                <BlockHeader>
                  <h4>External Condition</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    Depending on an external webservice recording can be enabled
                    or disabled.
                  </p>
                  <Input
                    label="Condition URI"
                    value={custom.condition_uri}
                    placeholder={
                      global.condition_uri
                        ? global.condition_uri
                        : 'The API for conditional recording (GET request).'
                    }
                    onChange={(value) =>
                      this.onUpdateField('', 'condition_uri', value, custom)
                    }
                  />
                </BlockBody>
              </Block>
            )}

            {/* Persistence block */}
            {showPersistenceSection && (
              <Block>
                <BlockHeader>
                  <h4>Persistence</h4>
                </BlockHeader>
                <BlockBody>
                  {loading && (
                    <InfoBar
                      type="loading"
                      message="Verifying your persistence settings."
                    />
                  )}
                  {verifyPersistenceSuccess && (
                    <InfoBar
                      type="success"
                      message="Persistence settings are successfully verified."
                    />
                  )}
                  {verifyPersistenceError && (
                    <InfoBar type="alert" message={verifyPersistenceMessage} />
                  )}
                  {persistenceSuccess && (
                    <InfoBar
                      type="success"
                      message="Persistence settings are successfully saved."
                    />
                  )}
                  {persistenceError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving your settings."
                    />
                  )}
                  <p>
                    Having the ability to store your recordings is the beginning
                    of everything. You can choose between our{' '}
                    <a
                      href="https://doc.kerberos.io/hub/license/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Hub (SAAS offering)
                    </a>{' '}
                    or your own{' '}
                    <a
                      href="https://doc.kerberos.io/vault/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Vault
                    </a>{' '}
                    deployment.
                  </p>
                  <Dropdown
                    isRadio
                    placeholder="Select a persistence"
                    selected={[global.cloud]}
                    items={this.storageTypes}
                    onChange={this.changeStorageType}
                  />

                  {global.cloud === this.KERBEROS_HUB && (
                    <>
                      <Input
                        label="Kerberos Hub API URL"
                        placeholder="The API endpoint for uploading your recordings."
                        value={global.s3.proxyuri}
                        onChange={(value) =>
                          this.changeS3Value('proxyuri', value)
                        }
                      />
                      <Input
                        label="Region"
                        placeholder="The region we are storing our recordings in."
                        value={global.s3.region}
                        onChange={(value) =>
                          this.changeS3Value('region', value)
                        }
                      />
                      <Input
                        label="Bucket"
                        placeholder="The bucket we are storing our recordings in."
                        value={global.s3.bucket}
                        onChange={(value) =>
                          this.changeS3Value('bucket', value)
                        }
                      />
                      <Input
                        label="Username/Directory"
                        placeholder="The username of your Kerberos Hub account."
                        value={global.s3.username}
                        onChange={(value) =>
                          this.changeS3Value('username', value)
                        }
                      />
                    </>
                  )}

                  {global.cloud === this.KERBEROS_VAULT && (
                    <>
                      <Input
                        label="Kerberos Vault API URL"
                        placeholder="The Kerberos Vault API"
                        value={global.kstorage.uri}
                        onChange={(value) =>
                          this.changeVaultValue('uri', value)
                        }
                      />
                      <Input
                        label="Provider"
                        placeholder="The provider to which your recordings will be send."
                        value={global.kstorage.provider}
                        onChange={(value) =>
                          this.changeVaultValue('provider', value)
                        }
                      />
                      <Input
                        label="Directory"
                        placeholder="Sub directory the recordings will be stored in your provider."
                        value={global.kstorage.directory}
                        onChange={(value) =>
                          this.changeVaultValue('directory', value)
                        }
                      />
                      <Input
                        label="Access key"
                        placeholder="The access key of your Kerberos Vault account."
                        value={global.kstorage.access_key}
                        onChange={(value) =>
                          this.changeVaultValue('access_key', value)
                        }
                      />
                      <Input
                        label="Secret key"
                        placeholder="The secret key of your Kerberos Vault account."
                        value={global.kstorage.secret_access_key}
                        onChange={(value) =>
                          this.changeVaultValue('secret_access_key', value)
                        }
                      />
                    </>
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Verify Connection"
                    disabled={loading}
                    onClick={this.verifyPersistenceSettings}
                    type={loading ? 'neutral' : 'default'}
                    icon="verify"
                  />
                  <Button
                    label="Save"
                    type="submit"
                    onClick={this.savePersistenceSettings}
                    buttonType="submit"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}
          </div>
        </div>
      </div>
    );
  }
}

const mapStateToProps = (state /* , ownProps */) => ({
  config: state.agent.config,
});

const mapDispatchToProps = (dispatch /* , ownProps */) => ({
  dispatchVerifyHub: (config, success, error) =>
    dispatch(verifyHub(config, success, error)),
  dispatchVerifyPersistence: (config, success, error) =>
    dispatch(verifyPersistence(config, success, error)),
  dispatchGetConfig: () => dispatch(getConfig()),
  dispatchSaveConfig: (config, success, error) =>
    dispatch(saveConfig(config, success, error)),
});

Settings.propTypes = {
  dispatchConfig: PropTypes.bool.isRequired,
};

export default withRouter(
  connect(mapStateToProps, mapDispatchToProps)(Settings)
);
