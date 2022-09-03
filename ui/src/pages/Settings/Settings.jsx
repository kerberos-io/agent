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
import { Link, withRouter } from 'react-router-dom';
import { connect } from 'react-redux';
import ImageCanvas from '../../components/ImageCanvas/ImageCanvas';
import './Settings.scss';
import timezones from './timezones';
import {
  addRegion,
  updateRegion,
  removeRegion,
  saveConfig,
  verifyHub,
  verifyPersistence,
  getConfig,
  updateConfig,
} from '../../actions/agent';

// eslint-disable-next-line react/prefer-stateless-function
class Settings extends React.Component {
  KERBEROS_VAULT = 'kstorage'; // @TODO needs to change

  KERBEROS_HUB = 's3'; // @TODO needs to change

  constructor() {
    super();
    this.state = {
      search: '',
      config: {},
      selectedTab: 'overview',
      mqttSuccess: false,
      mqttError: false,
      configSuccess: false,
      configError: false,
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

    this.changeTab = this.changeTab.bind(this);
    this.onUpdateField = this.onUpdateField.bind(this);
    this.onUpdateToggle = this.onUpdateToggle.bind(this);
    this.onUpdateNumberField = this.onUpdateNumberField.bind(this);
    this.onUpdateTimeline = this.onUpdateTimeline.bind(this);
    this.verifyPersistenceSettings = this.verifyPersistenceSettings.bind(this);
    this.verifyHubSettings = this.verifyHubSettings.bind(this);
    this.calculateTimetable = this.calculateTimetable.bind(this);
    this.saveConfig = this.saveConfig.bind(this);
    this.onUpdateDropdown = this.onUpdateDropdown.bind(this);
    this.onAddRegion = this.onAddRegion.bind(this);
    this.onUpdateRegion = this.onUpdateRegion.bind(this);
    this.onDeleteRegion = this.onDeleteRegion.bind(this);
  }

  componentDidMount() {
    const { dispatchGetConfig } = this.props;
    dispatchGetConfig((data) => {
      const { config } = data;
      this.setState((prevState) => ({
        ...prevState,
        config,
      }));
      this.calculateTimetable(config.timetable);
    });
  }

  componentWillUnmount() {
    document.removeEventListener('keydown', this.escFunction, false);
    clearInterval(this.interval);
  }

  onAddRegion(device, id, polygon) {
    const { dispatchAddRegion } = this.props;
    dispatchAddRegion(id, polygon);
  }

  onUpdateRegion(device, id, polygon) {
    const { dispatchUpdateRegion } = this.props;
    dispatchUpdateRegion(id, polygon);
  }

  onDeleteRegion(device, id, polygon) {
    const { dispatchRemoveRegion } = this.props;
    dispatchRemoveRegion(id, polygon);
  }

  onUpdateField(type, name, event, object) {
    const { value } = event.target;
    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, {
      ...object,
      [name]: value,
    });
  }

  onUpdateToggle(type, name, event, object) {
    const { checked } = event.target;
    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, {
      ...object,
      [name]: checked ? 'true' : 'false',
    });
  }

  onUpdateDropdown(type, name, value, object) {
    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, {
      ...object,
      [name]: value,
    });
  }

  onUpdateNumberField(type, name, event, object) {
    const { value } = event.target;
    let number = parseInt(value, 10);
    number = Number.isNaN(number) ? 0 : number;
    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, {
      ...object,
      [name]: number,
    });
  }

  onUpdateTimeline(type, index, key, event, object) {
    const { value } = event.target;
    let seconds = -1;
    if (value) {
      const a = value.split(':'); // split it at the colons
      if (a.length === 2) {
        if (a[0] <= 24 && a[1] < 60) {
          seconds = +a[0] * 60 * 60 + +a[1] * 60;
          this.timetable[index][`${key}Full`] = value;
        }
      }
    }

    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, [
      ...object.slice(0, index),
      {
        ...object[index],
        [key]: seconds,
      },
      ...object.slice(index + 1),
    ]);
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

  convertSecondsToHourMinute(seconds) {
    let sec = seconds;
    let hours = Math.floor(sec / 3600);
    hours = hours.toString();
    if (hours < 10) {
      hours = `0${hours}`;
    }
    if (hours.length > 2) {
      hours = hours.slice(0, 2);
    }
    sec %= 3600;
    let minutes = Math.floor(sec / 60);
    minutes = minutes.toString();
    if (minutes < 10) {
      minutes = `0${minutes}`;
    }
    if (minutes.length > 2) {
      minutes = minutes.slice(0, 2);
    }
    return `${hours}:${minutes}`;
  }

  changeTab(tab) {
    this.setState({
      selectedTab: tab,
    });
  }

  saveConfig() {
    const { config, dispatchSaveConfig } = this.props;

    this.setState({
      verifyPersistenceSuccess: false,
      verifyPersistenceError: false,
      verifyHubSuccess: false,
      verifyHubError: false,
      configSuccess: false,
      configError: false,
    });

    if (config) {
      dispatchSaveConfig(
        config.config,
        () => {
          this.setState({
            configSuccess: true,
            configError: false,
          });
        },
        () => {
          this.setState({
            configSuccess: false,
            configError: true,
          });
        }
      );
    }
  }

  verifyHubSettings() {
    const { config, dispatchVerifyHub } = this.props;
    if (config) {
      // overriding global for testing.
      // hub_key: "xxx"
      // hub_private_key: "xxxx"
      // hub_site: "testsite"
      // hub_uri: "https://api.cloud.kerberos.io"

      this.setState({
        configSuccess: false,
        configError: false,
        verifyPersistenceSuccess: false,
        verifyPersistenceError: false,
        verifyHubSuccess: false,
        verifyHubError: false,
        verifyHubErrorMessage: '',
        hubSuccess: false,
        hubError: false,
        loadingHub: true,
      });

      // .... test fields

      dispatchVerifyHub(
        config.config,
        () => {
          this.setState({
            verifyHubSuccess: true,
            verifyHubError: false,
            verifyHubErrorMessage: '',
            hubSuccess: false,
            hubError: false,
            loadingHub: false,
          });
        },
        (error) => {
          this.setState({
            verifyHubSuccess: false,
            verifyHubError: true,
            verifyHubErrorMessage: error,
            hubSuccess: false,
            hubError: false,
            loadingHub: false,
          });
        }
      );
    }
  }

  verifyPersistenceSettings() {
    const { config, dispatchVerifyPersistence } = this.props;
    if (config) {
      this.setState({
        configSuccess: false,
        configError: false,
        verifyHubSuccess: false,
        verifyHubError: false,
        verifyPersistenceSuccess: false,
        verifyPersistenceError: false,
        persistenceSuccess: false,
        persistenceError: false,
        loading: true,
      });

      dispatchVerifyPersistence(
        config.config,
        () => {
          this.setState({
            verifyPersistenceSuccess: true,
            verifyPersistenceError: false,
            verifyPersistenceMessage: '',
            persistenceSuccess: false,
            persistenceError: false,
            loading: false,
          });
        },
        (error) => {
          this.setState({
            verifyPersistenceSuccess: false,
            verifyPersistenceError: true,
            verifyPersistenceMessage: error,
            persistenceSuccess: false,
            persistenceError: false,
            loading: false,
          });
        }
      );
    }
  }

  render() {
    const {
      selectedTab,
      search,
      configSuccess,
      configError,
      verifyHubSuccess,
      verifyHubError,
      verifyHubErrorMessage,
      verifyPersistenceSuccess,
      verifyPersistenceError,
      verifyPersistenceMessage,
      loading,
      loadingHub,
    } = this.state;

    const { config: c } = this.props;
    const { config, snapshot } = c;

    const snapshotBase64 = 'data:image/png;base64,';
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

    return config ? (
      <div id="settings">
        <Breadcrumb title="Settings" level1="Onboard your camera" level1Link="">
          <Link to="/media">
            <Button label="Watch recordings" icon="media" type="default" />
          </Link>
        </Breadcrumb>
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
            {config.offline !== 'true' && (
              <Tab
                label="Streaming"
                value="streaming"
                active={selectedTab === 'streaming'}
                onClick={() => this.changeTab('streaming')}
                icon={<Icon label="livestream" />}
              />
            )}
            <Tab
              label="Conditions"
              value="conditions"
              active={selectedTab === 'conditions'}
              onClick={() => this.changeTab('conditions')}
              icon={<Icon label="activity" />}
            />
            {config.offline !== 'true' && (
              <Tab
                label="Persistence"
                value="persistence"
                active={selectedTab === 'persistence'}
                onClick={() => this.changeTab('persistence')}
                icon={<Icon label="cloud" />}
              />
            )}
          </Tabs>
        </ControlBar>

        {configSuccess && (
          <InfoBar
            type="success"
            message="Your configuration have been updated successfully."
          />
        )}
        {configError && (
          <InfoBar type="alert" message="Something went wrong while saving." />
        )}

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
        <div className="stats grid-container --two-columns">
          <div>
            {/* General settings block */}
            {showOverviewSection && (
              <Block>
                <BlockHeader>
                  <h4>General</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    General settings allow you to configure your Kerberos Agents
                    on a higher level.
                  </p>
                  <Input
                    noPadding
                    label="key"
                    disabled
                    defaultValue={config.key}
                  />

                  <Input
                    noPadding
                    label="camera name"
                    defaultValue={config.name}
                    onChange={(value) =>
                      this.onUpdateField('', 'name', value, config)
                    }
                  />

                  <Dropdown
                    isRadio
                    icon="world"
                    label="Timezone"
                    placeholder="Select a timezone"
                    items={this.timezones}
                    selected={[config.timezone]}
                    shorten
                    shortenType="end"
                    shortenMaxLength={30}
                    onChange={(value) =>
                      this.onUpdateDropdown('', 'timezone', value[0], config)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
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
                  <div className="warning-message">
                    <InfoBar
                      message="Currently only H264 RTSP streams are supported."
                      type="info"
                    />
                  </div>
                  <p>
                    Camera settings are required to make a connection to your
                    camera of choice.
                  </p>
                  <Input
                    noPadding
                    label="RTSP URL"
                    value={config.capture.ipcamera.rtsp}
                    placeholder="The IP camera address"
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'rtsp',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
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
                  <p>
                    General settings allow you to configure your Kerberos Agents
                    on a higher level.
                  </p>
                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.capture.continuous === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'continuous',
                          event,
                          config.capture
                        )
                      }
                    />
                    <div>
                      <span>Continuous recording</span>
                      <p>Make 24/7 or motion based recordings.</p>
                    </div>
                  </div>

                  <Input
                    noPadding
                    label="max video duration (seconds)"
                    value={config.capture.maxlengthrecording}
                    placeholder="The maximum duration of a recording."
                    onChange={(value) =>
                      this.onUpdateNumberField(
                        'capture',
                        'maxlengthrecording',
                        value,
                        config.capture
                      )
                    }
                  />

                  {config.capture.continuous !== 'true' && (
                    <>
                      <Input
                        noPadding
                        label="pre recording (seconds)"
                        value={config.capture.prerecording}
                        placeholder="Seconds before an event occurred."
                        onChange={(value) =>
                          this.onUpdateNumberField(
                            'capture',
                            'prerecording',
                            value,
                            config.capture
                          )
                        }
                      />
                      <Input
                        noPadding
                        label="post recording (seconds)"
                        value={config.capture.postrecording}
                        placeholder="Seconds after an event occurred."
                        onChange={(value) =>
                          this.onUpdateNumberField(
                            'capture',
                            'postrecording',
                            value,
                            config.capture
                          )
                        }
                      />
                      <Input
                        noPadding
                        label="Recording threshold (pixels)"
                        value={config.capture.pixelChangeThreshold}
                        placeholder="The number of pixels changed to record"
                        onChange={(value) =>
                          this.onUpdateNumberField(
                            'capture',
                            'pixelChangeThreshold',
                            value,
                            config.capture
                          )
                        }
                      />
                    </>
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* STUN/TURN block */}
            {showStreamingSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>STUN/TURN for WebRTC</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    For full-resolution livestreaming we use the concept of
                    WebRTC. One of the key capabilities is the ICE-candidate
                    feature, which allows NAT traversal using the concepts of
                    STUN/TURN.
                  </p>
                  <Input
                    noPadding
                    label="STUN server"
                    value={config.stunuri}
                    onChange={(value) =>
                      this.onUpdateField('', 'stunuri', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label="TURN server"
                    value={config.turnuri}
                    onChange={(value) =>
                      this.onUpdateField('', 'turnuri', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label="Username"
                    value={config.turn_username}
                    onChange={(value) =>
                      this.onUpdateField('', 'turn_username', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label="Password"
                    value={config.turn_password}
                    onChange={(value) =>
                      this.onUpdateField('', 'turn_password', value, config)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {showPersistenceSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>Kerberos Hub</h4>
                </BlockHeader>
                <BlockBody>
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
                    noPadding
                    label="API url"
                    placeholder="The API for Kerberos Hub."
                    value={config.hub_uri}
                    onChange={(value) =>
                      this.onUpdateField('', 'hub_uri', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label="Public key"
                    placeholder="The public key granted to your Kerberos Hub account."
                    value={config.hub_key}
                    onChange={(value) =>
                      this.onUpdateField('', 'hub_key', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label="Private key"
                    placeholder="The private key granted to your Kerberos Hub account."
                    value={config.hub_private_key}
                    onChange={(value) =>
                      this.onUpdateField('', 'hub_private_key', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label="Site"
                    value={config.hub_site}
                    placeholder="The site ID the Kerberos Agents are belonging to in Kerberos Hub."
                    onChange={(value) =>
                      this.onUpdateField('', 'hub_site', value, config)
                    }
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
                    onClick={this.saveConfig}
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
                  {config.region && (
                    <ImageCanvas
                      image={snapshotBase64 + snapshot}
                      polygons={config.region.polygon}
                      rendered={false}
                      onAddRegion={this.onAddRegion}
                      onUpdateRegion={this.onUpdateRegion}
                      onDeleteRegion={this.onDeleteRegion}
                    />
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}
          </div>

          <div>
            {/* General settings block */}
            {showCameraSection && (
              <Block>
                <BlockHeader>
                  <h4>ONVIF</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    Credentials to communicate with ONVIF capabilities. These
                    are used for PTZ or other capabilities provided by the
                    camera.
                  </p>

                  <Input
                    noPadding
                    label="onvif xaddr"
                    value={config.capture.ipcamera.onvif_xaddr}
                    placeholder="http://x.x.x.x/onvif/device_service"
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_xaddr',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />

                  <Input
                    noPadding
                    label="username"
                    value={config.capture.ipcamera.onvif_username}
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_username',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />

                  <Input
                    noPadding
                    label="password"
                    value={config.capture.ipcamera.onvif_password}
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_password',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* General settings block */}
            {showOverviewSection && (
              <Block>
                <BlockHeader>
                  <h4>Advanced configuration</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    Detailed configuration options to enable or disable specific
                    parts of the Kerberos Agent
                  </p>
                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.offline === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle('', 'offline', event, config)
                      }
                    />
                    <div>
                      <span>Offline mode</span>
                      <p>Disable all outgoing traffic</p>
                    </div>
                  </div>
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
                  />
                </BlockFooter>
              </Block>
            )}

            {showStreamingSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>MQTT</h4>
                </BlockHeader>
                <BlockBody>
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
                    noPadding
                    label="Broker Uri"
                    value={config.mqtturi}
                    onChange={(value) =>
                      this.onUpdateField('', 'mqtturi', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label="Username"
                    value={config.mqtt_username}
                    onChange={(value) =>
                      this.onUpdateField('', 'mqtt_username', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label="Password"
                    value={config.mqtt_password}
                    onChange={(value) =>
                      this.onUpdateField('', 'mqtt_password', value, config)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* STUN/TURN block */}
            {showStreamingSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>Forwarding and transcoding</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    Optimisations and enhancements for TURN/STUN communication.
                  </p>

                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.capture.forwardwebrtc === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'forwardwebrtc',
                          event,
                          config.capture
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
                      on={config.capture.transcodingwebrtc === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'transcodingwebrtc',
                          event,
                          config.capture
                        )
                      }
                    />
                    <div>
                      <span>Transcode stream</span>
                      <p>Convert stream to a lower resolution</p>
                    </div>
                  </div>

                  {config.capture.transcodingwebrtc === 'true' && (
                    <Input
                      noPadding
                      label="Downscale resolution (in % or original resolution)"
                      value={config.capture.transcodingresolution}
                      placeholder="The % of the original resolution."
                      onChange={(value) =>
                        this.onUpdateNumberField(
                          'capture',
                          'transcodingresolution',
                          value,
                          config.capture
                        )
                      }
                    />
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveConfig}
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
                  <p>
                    When recordings are fragmented they are suitable for an HLS
                    stream. When turned on the MP4 container will look a bit
                    different.
                  </p>

                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.capture.fragmented === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'fragmented',
                          event,
                          config.capture
                        )
                      }
                    />
                    <div>
                      <span>Enable fragmentation</span>
                      <p>Fragmented recordings are required for HLS.</p>
                    </div>
                  </div>

                  {config.capture.fragmented === 'true' && (
                    <Input
                      noPadding
                      label="fragment duration"
                      value={config.capture.fragmentedduration}
                      placeholder="Duration of a single fragment."
                      onChange={(value) =>
                        this.onUpdateNumberField(
                          'capture',
                          'fragmentedduration',
                          value,
                          config.capture
                        )
                      }
                    />
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
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
                        <p>
                          Only make recordings between specific time intervals
                          (based on Timezone).
                        </p>

                        <div className="toggle-wrapper">
                          <Toggle
                            on={config.time === 'true'}
                            disabled={false}
                            onClick={(event) =>
                              this.onUpdateToggle('', 'time', event, config)
                            }
                          />
                          <div>
                            <span>Enabled</span>
                            <p>If enabled you can specify time windows.</p>
                          </div>
                        </div>

                        {config.time === 'true' && (
                          <div>
                            <span className="time-of-interest">Sunday</span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[0].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    0,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[0].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    0,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[0].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    0,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[0].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    0,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">Monday</span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[1].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    1,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[1].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    1,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[1].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    1,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[1].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    1,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">Tuesday</span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[2].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    2,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[2].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    2,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[2].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    2,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[2].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    2,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">Wednesday</span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[3].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    3,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[3].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    3,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[3].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    3,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[3].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    3,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">Thursday</span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[4].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    4,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[4].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    4,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[4].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    4,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[4].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    4,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">Friday</span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[5].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    5,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[5].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    5,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[5].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    5,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[5].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    5,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">Saturday</span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[6].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    6,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[6].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    6,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[6].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    6,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[6].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    6,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </BlockBody>

                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveConfig}
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
                  <h4>External Condition</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    Depending on an external webservice recording can be enabled
                    or disabled.
                  </p>
                  <Input
                    noPadding
                    label="Condition URI"
                    value={config.condition_uri}
                    placeholder={
                      config.condition_uri
                        ? config.condition_uri
                        : 'The API for conditional recording (GET request).'
                    }
                    onChange={(value) =>
                      this.onUpdateField('', 'condition_uri', value, config)
                    }
                  />
                </BlockBody>

                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* Persistence block */}
            {showPersistenceSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>Persistence</h4>
                </BlockHeader>
                <BlockBody>
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
                    icon="cloud"
                    placeholder="Select a persistence"
                    selected={[config.cloud]}
                    items={this.storageTypes}
                    onChange={(value) =>
                      this.onUpdateDropdown('', 'cloud', value[0], config)
                    }
                  />
                  {config.cloud === this.KERBEROS_HUB && (
                    <>
                      <Input
                        noPadding
                        label="Kerberos Hub API URL"
                        placeholder="The API endpoint for uploading your recordings."
                        value={config.s3 ? config.s3.proxyuri : ''}
                        onChange={(value) =>
                          this.onUpdateField('s3', 'proxyuri', value, config.s3)
                        }
                      />
                      <Input
                        noPadding
                        label="Region"
                        placeholder="The region we are storing our recordings in."
                        value={config.s3 ? config.s3.region : ''}
                        onChange={(value) =>
                          this.onUpdateField('s3', 'region', value, config.s3)
                        }
                      />
                      <Input
                        noPadding
                        label="Bucket"
                        placeholder="The bucket we are storing our recordings in."
                        value={config.s3 ? config.s3.bucket : ''}
                        onChange={(value) =>
                          this.onUpdateField('s3', 'bucket', value, config.s3)
                        }
                      />
                      <Input
                        noPadding
                        label="Username/Directory"
                        placeholder="The username of your Kerberos Hub account."
                        value={config.s3 ? config.s3.username : ''}
                        onChange={(value) =>
                          this.onUpdateField('s3', 'username', value, config.s3)
                        }
                      />
                    </>
                  )}
                  {config.cloud === this.KERBEROS_VAULT && (
                    <>
                      <Input
                        noPadding
                        label="Kerberos Vault API URL"
                        placeholder="The Kerberos Vault API"
                        value={config.kstorage ? config.kstorage.uri : ''}
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'uri',
                            value,
                            config.kstorage
                          )
                        }
                      />
                      <Input
                        noPadding
                        label="Provider"
                        placeholder="The provider to which your recordings will be send."
                        value={config.kstorage ? config.kstorage.provider : ''}
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'provider',
                            value,
                            config.kstorage
                          )
                        }
                      />
                      <Input
                        noPadding
                        label="Directory"
                        placeholder="Sub directory the recordings will be stored in your provider."
                        value={config.kstorage ? config.kstorage.directory : ''}
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'directory',
                            value,
                            config.kstorage
                          )
                        }
                      />
                      <Input
                        noPadding
                        label="Access key"
                        placeholder="The access key of your Kerberos Vault account."
                        value={
                          config.kstorage ? config.kstorage.access_key : ''
                        }
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'access_key',
                            value,
                            config.kstorage
                          )
                        }
                      />
                      <Input
                        noPadding
                        label="Secret key"
                        placeholder="The secret key of your Kerberos Vault account."
                        value={
                          config.kstorage
                            ? config.kstorage.secret_access_key
                            : ''
                        }
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'secret_access_key',
                            value,
                            config.kstorage
                          )
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
                    onClick={this.saveConfig}
                    buttonType="submit"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}
          </div>
        </div>
      </div>
    ) : (
      <></>
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
  dispatchGetConfig: (callback) => dispatch(getConfig(callback)),
  dispatchUpdateConfig: (field, value) => dispatch(updateConfig(field, value)),
  dispatchSaveConfig: (config, success, error) =>
    dispatch(saveConfig(config, success, error)),
  dispatchAddRegion: (id, polygon) => dispatch(addRegion(id, polygon)),
  dispatchRemoveRegion: (id, polygon) => dispatch(removeRegion(id, polygon)),
  dispatchUpdateRegion: (id, polygon) => dispatch(updateRegion(id, polygon)),
});

Settings.propTypes = {
  config: PropTypes.objectOf(PropTypes.object).isRequired,
  dispatchVerifyHub: PropTypes.func.isRequired,
  dispatchVerifyPersistence: PropTypes.func.isRequired,
  dispatchGetConfig: PropTypes.func.isRequired,
  dispatchUpdateConfig: PropTypes.func.isRequired,
  dispatchSaveConfig: PropTypes.func.isRequired,
  dispatchAddRegion: PropTypes.func.isRequired,
  dispatchUpdateRegion: PropTypes.func.isRequired,
  dispatchRemoveRegion: PropTypes.func.isRequired,
};

export default withRouter(
  connect(mapStateToProps, mapDispatchToProps)(Settings)
);
